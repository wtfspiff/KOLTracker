package twitter

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
	"github.com/kol-tracker/pkg/extractor"
)

type Tweet struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	AuthorID  string    `json:"author_id"`
}

type Monitor struct {
	cfg          *config.Config
	store        *db.Store
	client       *http.Client
	lastTweetIDs map[string]string // handle -> last tweet ID
	onTokenFound func(kolID int64, tokenAddr string, chain config.Chain, mentionTime time.Time)
}

func NewMonitor(cfg *config.Config, store *db.Store) *Monitor {
	return &Monitor{
		cfg:          cfg,
		store:        store,
		client:       &http.Client{Timeout: 30 * time.Second},
		lastTweetIDs: make(map[string]string),
	}
}

func (m *Monitor) SetTokenCallback(fn func(kolID int64, tokenAddr string, chain config.Chain, mentionTime time.Time)) {
	m.onTokenFound = fn
}

// Run starts monitoring all configured KOL twitter handles
func (m *Monitor) Run(ctx context.Context) error {
	log.Info().Strs("handles", m.cfg.KOLTwitterHandles).Msg("starting twitter monitor")

	ticker := time.NewTicker(m.cfg.TwitterPollInterval)
	defer ticker.Stop()

	// Initial fetch
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

func (m *Monitor) pollAll(ctx context.Context) {
	for _, handle := range m.cfg.KOLTwitterHandles {
		if ctx.Err() != nil {
			return
		}

		kol, err := m.store.GetKOLByHandle(handle)
		if err != nil {
			// Create KOL profile if doesn't exist
			kolID, _ := m.store.UpsertKOL(handle, handle, "")
			kol = &db.KOLProfile{ID: kolID, TwitterHandle: handle}
		}

		tweets, err := m.fetchTweets(ctx, handle)
		if err != nil {
			log.Error().Err(err).Str("handle", handle).Msg("failed to fetch tweets")
			continue
		}

		for _, tweet := range tweets {
			m.processTweet(kol.ID, handle, tweet)
		}
	}
}

func (m *Monitor) fetchTweets(ctx context.Context, handle string) ([]Tweet, error) {
	// Try Twitter API v2 first
	if m.cfg.TwitterBearerToken != "" {
		tweets, err := m.fetchViaAPI(ctx, handle)
		if err == nil && len(tweets) > 0 {
			return tweets, nil
		}
		log.Debug().Err(err).Str("handle", handle).Msg("API fetch failed, trying nitter")
	}

	// Fallback to Nitter RSS
	return m.fetchViaNitter(ctx, handle)
}

func (m *Monitor) fetchViaAPI(ctx context.Context, handle string) ([]Tweet, error) {
	// Resolve user ID
	userURL := fmt.Sprintf("https://api.twitter.com/2/users/by/username/%s", handle)
	req, _ := http.NewRequestWithContext(ctx, "GET", userURL, nil)
	req.Header.Set("Authorization", "Bearer "+m.cfg.TwitterBearerToken)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var userData struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userData); err != nil || userData.Data.ID == "" {
		return nil, fmt.Errorf("user not found")
	}

	// Fetch tweets
	tweetsURL := fmt.Sprintf("https://api.twitter.com/2/users/%s/tweets?max_results=20&tweet.fields=created_at,text", userData.Data.ID)
	if sinceID, ok := m.lastTweetIDs[handle]; ok {
		tweetsURL += "&since_id=" + sinceID
	}

	req, _ = http.NewRequestWithContext(ctx, "GET", tweetsURL, nil)
	req.Header.Set("Authorization", "Bearer "+m.cfg.TwitterBearerToken)

	resp2, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		return nil, fmt.Errorf("tweets status %d", resp2.StatusCode)
	}

	var tweetsData struct {
		Data []struct {
			ID        string `json:"id"`
			Text      string `json:"text"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&tweetsData); err != nil {
		return nil, err
	}

	var tweets []Tweet
	for _, t := range tweetsData.Data {
		ts, _ := time.Parse(time.RFC3339, t.CreatedAt)
		tweets = append(tweets, Tweet{ID: t.ID, Text: t.Text, CreatedAt: ts})
	}

	return tweets, nil
}

// Nitter RSS fallback
type rssItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
}

type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func (m *Monitor) fetchViaNitter(ctx context.Context, handle string) ([]Tweet, error) {
	for _, instance := range m.cfg.NitterInstances {
		url := fmt.Sprintf("%s/%s/rss", strings.TrimRight(instance, "/"), handle)

		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")

		resp, err := m.client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		var feed rssFeed
		if err := xml.Unmarshal(body, &feed); err != nil {
			continue
		}

		var tweets []Tweet
		for _, item := range feed.Channel.Items {
			text := item.Description
			text = htmlTagRe.ReplaceAllString(text, " ")
			text = html.UnescapeString(text)
			text = strings.TrimSpace(text)

			if text == "" && item.Title != "" {
				text = item.Title
			}

			// Extract tweet ID from link
			tweetID := ""
			parts := strings.Split(strings.TrimRight(item.Link, "/"), "/")
			if len(parts) > 0 {
				tweetID = parts[len(parts)-1]
				// Remove # fragment
				if idx := strings.Index(tweetID, "#"); idx > 0 {
					tweetID = tweetID[:idx]
				}
			}

			ts, _ := time.Parse(time.RFC1123Z, item.PubDate)
			if ts.IsZero() {
				ts = time.Now().UTC()
			}

			if text != "" {
				tweets = append(tweets, Tweet{ID: tweetID, Text: text, CreatedAt: ts})
			}
		}

		if len(tweets) > 0 {
			log.Info().Str("handle", handle).Int("count", len(tweets)).Str("via", instance).Msg("fetched tweets via nitter")
			return tweets, nil
		}
	}

	return nil, fmt.Errorf("all nitter instances failed for @%s", handle)
}

func (m *Monitor) processTweet(kolID int64, handle string, tweet Tweet) {
	// Skip if already seen
	if lastID, ok := m.lastTweetIDs[handle]; ok && tweet.ID <= lastID {
		return
	}
	if tweet.ID > m.lastTweetIDs[handle] {
		m.lastTweetIDs[handle] = tweet.ID
	}

	// Extract addresses, tokens, links
	result := extractor.Extract(tweet.Text)

	if !result.HasContent() {
		return
	}

	log.Info().
		Str("handle", handle).
		Str("tweet_id", tweet.ID).
		Int("tokens", len(result.AllTokenCAs())).
		Int("wallets", len(result.AllAddresses())).
		Strs("tickers", result.TokenSymbols).
		Msg("📱 tweet with content")

	// Store the post
	allTokenCAs := result.AllTokenCAs()
	allAddrs := result.AllAddresses()
	allLinks := concat(result.DexScreenerLinks, result.BirdeyeLinks, result.PumpFunLinks,
		result.PhotonLinks, result.GmgnLinks, result.BullxLinks)

	postID, _ := m.store.InsertPost(kolID, "twitter", tweet.ID, tweet.Text,
		tweet.CreatedAt, allTokenCAs, allAddrs, allLinks)

	// Store token mentions and trigger fresh buyer watch
	for _, ca := range allTokenCAs {
		chain := extractor.ClassifyAddress(ca)
		_ = m.store.InsertTokenMention(kolID, postID, ca, "", chain, tweet.CreatedAt)

		if m.onTokenFound != nil {
			m.onTokenFound(kolID, ca, chain, tweet.CreatedAt)
		}
	}

	for _, symbol := range result.TokenSymbols {
		_ = m.store.InsertTokenMention(kolID, postID, "", symbol, config.ChainSolana, tweet.CreatedAt)
	}

	// Store discovered wallet addresses (non-CA addresses from tweets)
	tokenCASet := make(map[string]bool)
	for _, ca := range allTokenCAs {
		tokenCASet[ca] = true
	}

	for _, addr := range result.SolanaAddresses {
		if !tokenCASet[addr] {
			m.store.UpsertWallet(kolID, addr, config.ChainSolana, "from_tweet", 0.7,
				fmt.Sprintf("tweet:%s", tweet.ID))
			log.Info().Str("address", abbrev(addr)).Str("source", "tweet").Msg("🔑 potential wallet found")
		}
	}
	for _, addr := range result.EVMAddresses {
		if !tokenCASet[addr] {
			m.store.UpsertWallet(kolID, addr, config.ChainEthereum, "from_tweet", 0.7,
				fmt.Sprintf("tweet:%s", tweet.ID))
		}
	}

	// Store detected bot preferences
	for botName := range result.BotSignals {
		log.Debug().Str("bot", botName).Str("handle", handle).Msg("bot reference detected")
	}
}

func abbrev(addr string) string {
	if len(addr) > 12 {
		return addr[:6] + "..." + addr[len(addr)-4:]
	}
	return addr
}

func concat(slices ...[]string) []string {
	var r []string
	for _, s := range slices {
		r = append(r, s...)
	}
	return r
}
