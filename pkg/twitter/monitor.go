package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	twitterscraper "github.com/imperatrona/twitter-scraper"
	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
	"github.com/kol-tracker/pkg/extractor"
)

// Monitor uses the reverse-engineered Twitter frontend API (imperatrona/twitter-scraper)
// to fetch tweets in real-time and backfill historical posts.
type Monitor struct {
	cfg          *config.Config
	store        *db.Store
	scraper      *twitterscraper.Scraper
	mu           sync.RWMutex
	lastTweetIDs map[string]string // handle -> last seen tweet ID
	seenTweets   map[string]bool   // tweet ID -> processed
	onTokenFound func(kolID int64, tokenAddr string, chain config.Chain, mentionTime time.Time)
	loggedIn     bool
}

func NewMonitor(cfg *config.Config, store *db.Store) *Monitor {
	return &Monitor{
		cfg:          cfg,
		store:        store,
		scraper:      twitterscraper.New(),
		lastTweetIDs: make(map[string]string),
		seenTweets:   make(map[string]bool),
	}
}

func (m *Monitor) SetTokenCallback(fn func(kolID int64, tokenAddr string, chain config.Chain, mentionTime time.Time)) {
	m.onTokenFound = fn
}

// Login authenticates the scraper using cookies, auth tokens, or username/password.
func (m *Monitor) Login() error {
	// Priority 1: Use saved cookies from file
	if m.cfg.TwitterCookieFile != "" {
		if data, err := os.ReadFile(m.cfg.TwitterCookieFile); err == nil {
			var cookies []*http.Cookie
			if json.Unmarshal(data, &cookies) == nil && len(cookies) > 0 {
				m.scraper.SetCookies(cookies)
				if m.scraper.IsLoggedIn() {
					m.loggedIn = true
					log.Info().Msg("🐦 twitter: logged in via saved cookies")
					return nil
				}
			}
		}
	}

	// Priority 2: Use auth_token + ct0 (extracted from browser)
	if m.cfg.TwitterAuthToken != "" && m.cfg.TwitterCSRFToken != "" {
		m.scraper.SetAuthToken(twitterscraper.AuthToken{
			Token:     m.cfg.TwitterAuthToken,
			CSRFToken: m.cfg.TwitterCSRFToken,
		})
		if m.scraper.IsLoggedIn() {
			m.loggedIn = true
			m.saveCookies()
			log.Info().Msg("🐦 twitter: logged in via auth token")
			return nil
		}
	}

	// Priority 3: Username + Password login
	if m.cfg.TwitterUsername != "" && m.cfg.TwitterPassword != "" {
		var err error
		if m.cfg.TwitterEmail != "" {
			err = m.scraper.Login(m.cfg.TwitterUsername, m.cfg.TwitterPassword, m.cfg.TwitterEmail)
		} else {
			err = m.scraper.Login(m.cfg.TwitterUsername, m.cfg.TwitterPassword)
		}
		if err != nil {
			return fmt.Errorf("twitter login failed: %w", err)
		}
		if m.scraper.IsLoggedIn() {
			m.loggedIn = true
			m.saveCookies()
			log.Info().Msg("🐦 twitter: logged in via username/password")
			return nil
		}
	}

	return fmt.Errorf("no twitter credentials configured (need TWITTER_USERNAME+PASSWORD or TWITTER_AUTH_TOKEN+CSRF_TOKEN)")
}

func (m *Monitor) saveCookies() {
	if m.cfg.TwitterCookieFile == "" {
		return
	}
	cookies := m.scraper.GetCookies()
	data, err := json.Marshal(cookies)
	if err != nil {
		return
	}
	os.WriteFile(m.cfg.TwitterCookieFile, data, 0600)
}

// Run starts the real-time polling loop.
func (m *Monitor) Run(ctx context.Context) error {
	if err := m.Login(); err != nil {
		log.Error().Err(err).Msg("twitter login failed - twitter monitoring disabled")
		// Block instead of returning error, so other goroutines keep running
		<-ctx.Done()
		return ctx.Err()
	}

	log.Info().Strs("handles", m.cfg.KOLTwitterHandles).Msg("🐦 twitter monitor started (private API)")

	ticker := time.NewTicker(m.cfg.TwitterPollInterval)
	defer ticker.Stop()

	// Initial poll
	m.pollAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			m.pollAll(ctx)
		}
	}
}

// BackfillKOL fetches historical tweets for a newly added KOL.
// Called immediately when a KOL is added via the frontend.
func (m *Monitor) BackfillKOL(ctx context.Context, kolID int64, handle string, maxTweets int) error {
	if !m.loggedIn {
		if err := m.Login(); err != nil {
			return err
		}
	}

	log.Info().Str("handle", handle).Int("max", maxTweets).Msg("🐦 backfilling tweets for new KOL")

	count := 0
	for tweet := range m.scraper.GetTweets(ctx, handle, maxTweets) {
		if tweet.Error != nil {
			log.Debug().Err(tweet.Error).Str("handle", handle).Msg("backfill tweet error")
			continue
		}

		tweetID := tweet.ID
		if m.seenTweets[tweetID] {
			continue
		}
		m.seenTweets[tweetID] = true

		ts := tweet.TimeParsed
		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		m.processTweet(kolID, handle, tweetID, tweet.Text, ts)
		count++

		// Rate limit: don't hammer the API
		if count%20 == 0 {
			time.Sleep(2 * time.Second)
		}
	}

	log.Info().Str("handle", handle).Int("processed", count).Msg("🐦 backfill complete")
	return nil
}

func (m *Monitor) pollAll(ctx context.Context) {
	m.mu.RLock()
	handles := make([]string, len(m.cfg.KOLTwitterHandles))
	copy(handles, m.cfg.KOLTwitterHandles)
	m.mu.RUnlock()

	for _, handle := range handles {
		if ctx.Err() != nil {
			return
		}

		kol, err := m.store.GetKOLByHandle(handle)
		if err != nil {
			kolID, _ := m.store.UpsertKOL(handle, handle, "")
			kol = &db.KOLProfile{ID: kolID, TwitterHandle: handle}
		}

		m.fetchNewTweets(ctx, kol.ID, handle)
	}
}

func (m *Monitor) fetchNewTweets(ctx context.Context, kolID int64, handle string) {
	if !m.loggedIn {
		return
	}

	// GetTweets returns a channel, fetch latest 20
	count := 0
	for tweet := range m.scraper.GetTweets(ctx, handle, 20) {
		if tweet.Error != nil {
			log.Debug().Err(tweet.Error).Str("handle", handle).Msg("fetch error")
			break
		}

		tweetID := tweet.ID

		// Skip already seen
		m.mu.RLock()
		seen := m.seenTweets[tweetID]
		lastID := m.lastTweetIDs[handle]
		m.mu.RUnlock()

		if seen || (lastID != "" && tweetID <= lastID) {
			continue
		}

		m.mu.Lock()
		m.seenTweets[tweetID] = true
		if tweetID > m.lastTweetIDs[handle] {
			m.lastTweetIDs[handle] = tweetID
		}
		m.mu.Unlock()

		ts := tweet.TimeParsed
		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		m.processTweet(kolID, handle, tweetID, tweet.Text, ts)
		count++
	}

	if count > 0 {
		log.Info().Str("handle", handle).Int("new_tweets", count).Msg("🐦 polled")
	}
}

// processTweet extracts wallet addresses, token CAs, and links from a tweet.
func (m *Monitor) processTweet(kolID int64, handle string, tweetID string, text string, ts time.Time) {
	result := extractor.Extract(text)

	if !result.HasContent() {
		return
	}

	log.Info().
		Str("handle", handle).
		Str("tweet_id", tweetID).
		Int("tokens", len(result.AllTokenCAs())).
		Int("wallets", len(result.AllAddresses())).
		Strs("tickers", result.TokenSymbols).
		Msg("📱 tweet with content")

	allTokenCAs := result.AllTokenCAs()
	allAddrs := result.AllAddresses()
	allLinks := concatSlices(result.DexScreenerLinks, result.BirdeyeLinks, result.PumpFunLinks,
		result.PhotonLinks, result.GmgnLinks, result.BullxLinks)

	postID, _ := m.store.InsertPost(kolID, "twitter", tweetID, text, ts, allTokenCAs, allAddrs, allLinks)

	// Token mentions → trigger fresh buyer watch
	for _, ca := range allTokenCAs {
		chain := extractor.ClassifyAddress(ca)
		_ = m.store.InsertTokenMention(kolID, postID, ca, "", chain, ts)
		if m.onTokenFound != nil {
			m.onTokenFound(kolID, ca, chain, ts)
		}
	}
	for _, symbol := range result.TokenSymbols {
		_ = m.store.InsertTokenMention(kolID, postID, "", symbol, config.ChainSolana, ts)
	}

	// Discovered wallet addresses (non-CA)
	tokenCASet := make(map[string]bool)
	for _, ca := range allTokenCAs {
		tokenCASet[ca] = true
	}
	for _, addr := range result.SolanaAddresses {
		if !tokenCASet[addr] {
			m.store.UpsertWallet(kolID, addr, config.ChainSolana, "from_tweet", 0.7, fmt.Sprintf("tweet:%s", tweetID))
		}
	}
	for _, addr := range result.EVMAddresses {
		if !tokenCASet[addr] {
			m.store.UpsertWallet(kolID, addr, config.ChainEthereum, "from_tweet", 0.7, fmt.Sprintf("tweet:%s", tweetID))
		}
	}

	for botName := range result.BotSignals {
		log.Debug().Str("bot", botName).Str("handle", handle).Msg("bot reference detected")
	}
}

// AddHandle adds a new handle to monitor at runtime (called when KOL added via frontend).
func (m *Monitor) AddHandle(handle string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, h := range m.cfg.KOLTwitterHandles {
		if strings.EqualFold(h, handle) {
			return
		}
	}
	m.cfg.KOLTwitterHandles = append(m.cfg.KOLTwitterHandles, handle)
}

// IsLoggedIn returns whether the scraper has an active session.
func (m *Monitor) IsLoggedIn() bool {
	return m.loggedIn
}

func concatSlices(slices ...[]string) []string {
	var r []string
	for _, s := range slices {
		r = append(r, s...)
	}
	return r
}
