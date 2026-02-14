package telegram

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
	"github.com/kol-tracker/pkg/extractor"
)

// Monitor scrapes public Telegram channels via their web preview pages.
type Monitor struct {
	cfg        *config.Config
	store      *db.Store
	client     *http.Client
	mu         sync.RWMutex
	lastMsgIDs map[string]string // channel -> last msg ID
	seenMsgs   map[string]bool
	onTokenFound func(kolID int64, tokenAddr string, chain config.Chain, mentionTime time.Time)
}

func NewMonitor(cfg *config.Config, store *db.Store) *Monitor {
	return &Monitor{
		cfg:        cfg,
		store:      store,
		client:     &http.Client{Timeout: 30 * time.Second},
		lastMsgIDs: make(map[string]string),
		seenMsgs:   make(map[string]bool),
	}
}

func (m *Monitor) SetTokenCallback(fn func(kolID int64, tokenAddr string, chain config.Chain, mentionTime time.Time)) {
	m.onTokenFound = fn
}

func (m *Monitor) Run(ctx context.Context) error {
	log.Info().Strs("channels", m.cfg.KOLTelegramChannels).Msg("📨 telegram monitor started")

	ticker := time.NewTicker(m.cfg.TelegramPollInterval)
	defer ticker.Stop()

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

// BackfillChannel fetches historical messages from a public Telegram channel.
// Scrapes the t.me/s/ web preview which shows the last ~20 messages per page.
// We scroll back by fetching older pages using the "before" parameter.
func (m *Monitor) BackfillChannel(ctx context.Context, kolID int64, channel string, maxPages int) error {
	log.Info().Str("channel", channel).Int("pages", maxPages).Msg("📨 backfilling telegram channel")

	totalProcessed := 0
	beforeID := "" // empty = latest

	for page := 0; page < maxPages; page++ {
		if ctx.Err() != nil {
			break
		}

		messages, oldestID, err := m.fetchMessages(ctx, channel, beforeID)
		if err != nil {
			log.Debug().Err(err).Str("channel", channel).Msg("backfill page error")
			break
		}

		if len(messages) == 0 {
			break
		}

		for _, msg := range messages {
			if m.seenMsgs[msg.ID] {
				continue
			}
			m.seenMsgs[msg.ID] = true
			m.processMessage(kolID, channel, msg)
			totalProcessed++
		}

		if oldestID == "" || oldestID == beforeID {
			break // no more pages
		}
		beforeID = oldestID
		time.Sleep(time.Second) // rate limit
	}

	log.Info().Str("channel", channel).Int("processed", totalProcessed).Msg("📨 telegram backfill complete")
	return nil
}

func (m *Monitor) pollAll(ctx context.Context) {
	m.mu.RLock()
	channels := make([]string, len(m.cfg.KOLTelegramChannels))
	copy(channels, m.cfg.KOLTelegramChannels)
	m.mu.RUnlock()

	for _, channel := range channels {
		if ctx.Err() != nil {
			return
		}

		kol, err := m.store.GetKOLByHandle(channel)
		if err != nil {
			kolID, _ := m.store.UpsertKOL(channel, "", channel)
			kol = &db.KOLProfile{ID: kolID, TelegramChannel: channel}
		}

		messages, _, err := m.fetchMessages(ctx, channel, "")
		if err != nil {
			log.Debug().Err(err).Str("channel", channel).Msg("telegram fetch error")
			continue
		}

		newCount := 0
		for _, msg := range messages {
			m.mu.RLock()
			seen := m.seenMsgs[msg.ID]
			m.mu.RUnlock()
			if seen {
				continue
			}

			m.mu.Lock()
			m.seenMsgs[msg.ID] = true
			if msg.ID > m.lastMsgIDs[channel] {
				m.lastMsgIDs[channel] = msg.ID
			}
			m.mu.Unlock()

			m.processMessage(kol.ID, channel, msg)
			newCount++
		}

		if newCount > 0 {
			log.Info().Str("channel", channel).Int("new", newCount).Msg("📨 polled telegram")
		}
	}
}

type TGMessage struct {
	ID        string
	Text      string
	Timestamp time.Time
}

var (
	msgIDRe   = regexp.MustCompile(`data-post="[^/]+/(\d+)"`)
	msgTextRe = regexp.MustCompile(`<div class="tgme_widget_message_text[^"]*"[^>]*>(.*?)</div>`)
	timeRe    = regexp.MustCompile(`datetime="([^"]+)"`)
	htmlTagRe = regexp.MustCompile(`<[^>]+>`)
)

func (m *Monitor) fetchMessages(ctx context.Context, channel string, beforeID string) ([]TGMessage, string, error) {
	url := fmt.Sprintf("https://t.me/s/%s", channel)
	if beforeID != "" {
		url += fmt.Sprintf("?before=%s", beforeID)
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	bodyStr := string(body)
	ids := msgIDRe.FindAllStringSubmatch(bodyStr, -1)
	texts := msgTextRe.FindAllStringSubmatch(bodyStr, -1)
	times := timeRe.FindAllStringSubmatch(bodyStr, -1)

	var messages []TGMessage
	oldestID := ""

	for i := 0; i < len(ids) && i < len(texts); i++ {
		msgID := ids[i][1]
		text := texts[i][1]
		text = htmlTagRe.ReplaceAllString(text, " ")
		text = html.UnescapeString(text)
		text = strings.TrimSpace(text)

		var ts time.Time
		if i < len(times) {
			ts, _ = time.Parse(time.RFC3339, times[i][1])
		}
		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		if text != "" {
			messages = append(messages, TGMessage{ID: msgID, Text: text, Timestamp: ts})
		}

		if oldestID == "" || msgID < oldestID {
			oldestID = msgID
		}
	}

	return messages, oldestID, nil
}

func (m *Monitor) processMessage(kolID int64, channel string, msg TGMessage) {
	result := extractor.Extract(msg.Text)
	if !result.HasContent() {
		return
	}

	log.Info().Str("channel", channel).Str("msg_id", msg.ID).
		Int("tokens", len(result.AllTokenCAs())).Msg("📨 TG message with content")

	allTokenCAs := result.AllTokenCAs()
	allAddrs := result.AllAddresses()
	allLinks := concatSlices(result.DexScreenerLinks, result.BirdeyeLinks, result.PumpFunLinks,
		result.PhotonLinks, result.GmgnLinks, result.BullxLinks)

	postID, _ := m.store.InsertPost(kolID, "telegram", msg.ID, msg.Text,
		msg.Timestamp, allTokenCAs, allAddrs, allLinks)

	for _, ca := range allTokenCAs {
		chain := extractor.ClassifyAddress(ca)
		_ = m.store.InsertTokenMention(kolID, postID, ca, "", chain, msg.Timestamp)
		if m.onTokenFound != nil {
			m.onTokenFound(kolID, ca, chain, msg.Timestamp)
		}
	}

	for _, symbol := range result.TokenSymbols {
		_ = m.store.InsertTokenMention(kolID, postID, "", symbol, config.ChainSolana, msg.Timestamp)
	}

	tokenCASet := make(map[string]bool)
	for _, ca := range allTokenCAs {
		tokenCASet[ca] = true
	}
	for _, addr := range result.SolanaAddresses {
		if !tokenCASet[addr] {
			m.store.UpsertWallet(kolID, addr, config.ChainSolana, "from_telegram", 0.6, fmt.Sprintf("tg:%s", msg.ID))
		}
	}
	for _, addr := range result.EVMAddresses {
		if !tokenCASet[addr] {
			m.store.UpsertWallet(kolID, addr, config.ChainEthereum, "from_telegram", 0.6, fmt.Sprintf("tg:%s", msg.ID))
		}
	}
}

// AddChannel adds a new channel to monitor at runtime.
func (m *Monitor) AddChannel(channel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.cfg.KOLTelegramChannels {
		if strings.EqualFold(c, channel) {
			return
		}
	}
	m.cfg.KOLTelegramChannels = append(m.cfg.KOLTelegramChannels, channel)
}

func concatSlices(slices ...[]string) []string {
	var r []string
	for _, s := range slices {
		r = append(r, s...)
	}
	return r
}
