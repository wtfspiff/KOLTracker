package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
	"github.com/kol-tracker/pkg/extractor"
)

// Monitor watches Telegram channels for token calls and wallet addresses.
// Uses either MTProto (gotd/td) or Bot API polling depending on config.
// For channels, we use an HTTP scraper against public channel web previews as fallback.
type Monitor struct {
	cfg          *config.Config
	store        *db.Store
	client       *http.Client
	lastMsgIDs   map[string]int64 // channel -> last message ID
	onTokenFound func(kolID int64, tokenAddr string, chain config.Chain, mentionTime time.Time)
}

func NewMonitor(cfg *config.Config, store *db.Store) *Monitor {
	return &Monitor{
		cfg:        cfg,
		store:      store,
		client:     &http.Client{Timeout: 30 * time.Second},
		lastMsgIDs: make(map[string]int64),
	}
}

func (m *Monitor) SetTokenCallback(fn func(kolID int64, tokenAddr string, chain config.Chain, mentionTime time.Time)) {
	m.onTokenFound = fn
}

// Run starts monitoring configured Telegram channels
func (m *Monitor) Run(ctx context.Context) error {
	if len(m.cfg.KOLTelegramChannels) == 0 {
		log.Info().Msg("no telegram channels configured, skipping")
		return nil
	}

	log.Info().Strs("channels", m.cfg.KOLTelegramChannels).Msg("starting telegram monitor")

	// Initial fetch of recent messages
	for _, ch := range m.cfg.KOLTelegramChannels {
		m.fetchChannelMessages(ctx, ch)
	}

	ticker := time.NewTicker(m.cfg.TelegramPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			for _, ch := range m.cfg.KOLTelegramChannels {
				m.fetchChannelMessages(ctx, ch)
			}
		}
	}
}

// fetchChannelMessages fetches recent messages from a public Telegram channel
// using the t.me/s/ web preview (no auth required for public channels)
func (m *Monitor) fetchChannelMessages(ctx context.Context, channel string) {
	kol, err := m.store.GetKOLByHandle(channel)
	if err != nil {
		kolID, _ := m.store.UpsertKOL(channel, "", channel)
		kol = &db.KOLProfile{ID: kolID, TelegramChannel: channel}
	}

	// Use Telegram's public web view for public channels
	messages, err := m.scrapePublicChannel(ctx, channel)
	if err != nil {
		log.Debug().Err(err).Str("channel", channel).Msg("public channel scrape failed")
		return
	}

	for _, msg := range messages {
		m.processMessage(kol.ID, channel, msg)
	}
}

type TGMessage struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// scrapePublicChannel fetches messages from t.me/s/channel_name
// This works for public channels without any authentication
func (m *Monitor) scrapePublicChannel(ctx context.Context, channel string) ([]TGMessage, error) {
	url := fmt.Sprintf("https://t.me/s/%s", channel)

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseTelegramHTML(string(body), channel)
}

// parseTelegramHTML extracts messages from t.me/s/ HTML
// This is a simplified parser - the HTML structure has
// div.tgme_widget_message_text containing the message text
func parseTelegramHTML(htmlContent, channel string) ([]TGMessage, error) {
	var messages []TGMessage

	// Quick regex-based extraction of message blocks
	// In production, use a proper HTML parser (goquery)
	import_re_needed := false
	_ = import_re_needed

	// Split by message containers
	// Each message is in: <div class="tgme_widget_message_wrap ..." data-post="channel/msgid">
	// The text is in: <div class="tgme_widget_message_text ...">...</div>
	// Timestamps in: <time datetime="...">

	// Simple approach: extract data-post IDs and text blocks
	content := htmlContent
	msgStart := 0
	for {
		// Find data-post attribute
		postIdx := indexFrom(content, `data-post="`, msgStart)
		if postIdx < 0 {
			break
		}
		postStart := postIdx + len(`data-post="`)
		postEnd := indexFrom(content, `"`, postStart)
		if postEnd < 0 {
			break
		}
		postValue := content[postStart:postEnd]

		// Extract message ID from "channel/123"
		var msgID int64
		if parts := splitLast(postValue, "/"); parts != "" {
			fmt.Sscanf(parts, "%d", &msgID)
		}

		// Find the message text div after this point
		textDivStart := indexFrom(content, `class="tgme_widget_message_text"`, postEnd)
		if textDivStart < 0 || textDivStart-postEnd > 5000 { // sanity check
			msgStart = postEnd + 1
			continue
		}

		// Find the opening > of this div
		textContentStart := indexFrom(content, ">", textDivStart)
		if textContentStart < 0 {
			msgStart = postEnd + 1
			continue
		}
		textContentStart++

		// Find the closing </div>
		textContentEnd := indexFrom(content, "</div>", textContentStart)
		if textContentEnd < 0 {
			msgStart = postEnd + 1
			continue
		}

		rawText := content[textContentStart:textContentEnd]
		// Strip HTML tags
		text := stripHTML(rawText)

		// Find timestamp
		timeIdx := indexFrom(content, `datetime="`, postEnd)
		var ts time.Time
		if timeIdx > 0 && timeIdx < textContentEnd+500 {
			tsStart := timeIdx + len(`datetime="`)
			tsEnd := indexFrom(content, `"`, tsStart)
			if tsEnd > 0 {
				ts, _ = time.Parse(time.RFC3339, content[tsStart:tsEnd])
			}
		}
		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		if text != "" && msgID > 0 {
			messages = append(messages, TGMessage{
				ID:        msgID,
				Text:      text,
				Timestamp: ts,
			})
		}

		msgStart = textContentEnd + 1
	}

	return messages, nil
}

func (m *Monitor) processMessage(kolID int64, channel string, msg TGMessage) {
	// Skip already seen
	if msg.ID <= m.lastMsgIDs[channel] {
		return
	}
	m.lastMsgIDs[channel] = msg.ID

	// Extract content
	result := extractor.Extract(msg.Text)
	if !result.HasContent() {
		return
	}

	postIDStr := fmt.Sprintf("%s:%d", channel, msg.ID)

	log.Info().
		Str("channel", channel).
		Int64("msg_id", msg.ID).
		Int("tokens", len(result.AllTokenCAs())).
		Int("wallets", len(result.AllAddresses())).
		Strs("tickers", result.TokenSymbols).
		Msg("📨 telegram message with content")

	allTokenCAs := result.AllTokenCAs()
	allAddrs := result.AllAddresses()
	allLinks := mergeLinks(result)

	postDBID, _ := m.store.InsertPost(kolID, "telegram", postIDStr, msg.Text,
		msg.Timestamp, allTokenCAs, allAddrs, allLinks)

	// Token mentions
	tokenCASet := make(map[string]bool)
	for _, ca := range allTokenCAs {
		tokenCASet[ca] = true
		chain := extractor.ClassifyAddress(ca)
		_ = m.store.InsertTokenMention(kolID, postDBID, ca, "", chain, msg.Timestamp)

		if m.onTokenFound != nil {
			m.onTokenFound(kolID, ca, chain, msg.Timestamp)
		}
	}

	for _, symbol := range result.TokenSymbols {
		_ = m.store.InsertTokenMention(kolID, postDBID, "", symbol, config.ChainSolana, msg.Timestamp)
	}

	// Discovered wallets
	for _, addr := range result.SolanaAddresses {
		if !tokenCASet[addr] {
			m.store.UpsertWallet(kolID, addr, config.ChainSolana, "from_telegram", 0.6,
				fmt.Sprintf("telegram:%s", postIDStr))
		}
	}
	for _, addr := range result.EVMAddresses {
		if !tokenCASet[addr] {
			m.store.UpsertWallet(kolID, addr, config.ChainEthereum, "from_telegram", 0.6,
				fmt.Sprintf("telegram:%s", postIDStr))
		}
	}

	// Log bot signals
	if len(result.BotSignals) > 0 {
		botNames := make([]string, 0, len(result.BotSignals))
		for k := range result.BotSignals {
			botNames = append(botNames, k)
		}
		log.Debug().Strs("bots", botNames).Str("channel", channel).Msg("bot references in TG")
	}
}

// helpers

func indexFrom(s, substr string, start int) int {
	if start >= len(s) {
		return -1
	}
	idx := indexOf(s[start:], substr)
	if idx < 0 {
		return -1
	}
	return start + idx
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func splitLast(s, sep string) string {
	idx := -1
	for i := len(s) - 1; i >= 0; i-- {
		if string(s[i]) == sep {
			idx = i
			break
		}
	}
	if idx >= 0 && idx < len(s)-1 {
		return s[idx+1:]
	}
	return ""
}

func stripHTML(s string) string {
	// Replace <br> with newline
	for _, br := range []string{"<br>", "<br/>", "<br />"} {
		s = replaceAll(s, br, "\n")
	}
	// Strip all HTML tags
	inTag := false
	var result []byte
	for _, c := range []byte(s) {
		if c == '<' {
			inTag = true
		} else if c == '>' {
			inTag = false
		} else if !inTag {
			result = append(result, c)
		}
	}
	// Decode HTML entities
	decoded := string(result)
	decoded = replaceAll(decoded, "&amp;", "&")
	decoded = replaceAll(decoded, "&lt;", "<")
	decoded = replaceAll(decoded, "&gt;", ">")
	decoded = replaceAll(decoded, "&quot;", "\"")
	decoded = replaceAll(decoded, "&#39;", "'")
	return trimSpace(decoded)
}

func replaceAll(s, old, new string) string {
	for {
		idx := indexOf(s, old)
		if idx < 0 {
			return s
		}
		s = s[:idx] + new + s[idx+len(old):]
	}
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func mergeLinks(r *db.ExtractionResult) []string {
	var all []string
	all = append(all, r.DexScreenerLinks...)
	all = append(all, r.BirdeyeLinks...)
	all = append(all, r.PumpFunLinks...)
	all = append(all, r.PhotonLinks...)
	all = append(all, r.GmgnLinks...)
	all = append(all, r.BullxLinks...)
	return all
}

func _unused() {
	_ = json.Marshal
}
