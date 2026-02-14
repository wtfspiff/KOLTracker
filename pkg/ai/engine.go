package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
)

// Engine wraps an LLM (Claude / OpenAI / local) to provide intelligent analysis
// that goes far beyond regex + heuristics.
type Engine struct {
	cfg    *config.Config
	store  *db.Store
	client *http.Client

	provider   string // "anthropic", "openai", "ollama"
	apiKey     string
	model      string
	apiBaseURL string
}

func NewEngine(cfg *config.Config, store *db.Store) *Engine {
	e := &Engine{
		cfg:    cfg,
		store:  store,
		client: &http.Client{Timeout: 120 * time.Second},
	}

	// Determine provider from config
	if cfg.AnthropicAPIKey != "" {
		e.provider = "anthropic"
		e.apiKey = cfg.AnthropicAPIKey
		e.model = envOr("AI_MODEL", "claude-sonnet-4-20250514")
		e.apiBaseURL = "https://api.anthropic.com/v1/messages"
	} else if cfg.OpenAIAPIKey != "" {
		e.provider = "openai"
		e.apiKey = cfg.OpenAIAPIKey
		e.model = envOr("AI_MODEL", "gpt-4o")
		e.apiBaseURL = "https://api.openai.com/v1/chat/completions"
	} else if cfg.OllamaURL != "" {
		e.provider = "ollama"
		e.model = envOr("AI_MODEL", "llama3.1")
		e.apiBaseURL = cfg.OllamaURL + "/api/chat"
	}

	if e.provider != "" {
		log.Info().Str("provider", e.provider).Str("model", e.model).Msg("🤖 AI engine initialized")
	} else {
		log.Warn().Msg("⚠️ No AI provider configured - AI features disabled")
	}

	return e
}

func (e *Engine) IsEnabled() bool {
	return e.provider != ""
}

// ============================================================================
// 1. INTELLIGENT SOCIAL POST ANALYSIS
// ============================================================================

// AnalyzeSocialPost uses the LLM to deeply understand a KOL's post.
// Extracts: intent (buy/sell/shill/alert), sentiment, token references,
// wallet addresses with context (is it their wallet? a dev wallet? a contract?),
// trading bot mentions, and whether this looks like a paid promotion.
type PostAnalysis struct {
	Intent            string            `json:"intent"`             // "buy_call","sell_alert","shill","analysis","meme","warning"
	Sentiment         string            `json:"sentiment"`          // "extremely_bullish","bullish","neutral","bearish","fud"
	Tokens            []TokenRef        `json:"tokens"`
	Wallets           []WalletRef       `json:"wallets"`
	TradingBots       []string          `json:"trading_bots"`       // detected bot names
	IsPaidPromo       bool              `json:"is_paid_promo"`
	Urgency           string            `json:"urgency"`            // "high","medium","low"
	BuyBeforePost     bool              `json:"buy_before_post"`    // LLM thinks KOL bought before posting
	ExpectedAction    string            `json:"expected_action"`    // "will_buy","already_bought","selling","observing"
	RiskOfWashTrading string            `json:"risk_of_wash"`       // "high","medium","low"
	Reasoning         string            `json:"reasoning"`
	ConfidenceScore   float64           `json:"confidence_score"`
}

type TokenRef struct {
	Address    string `json:"address"`
	Symbol     string `json:"symbol"`
	Chain      string `json:"chain"`
	Context    string `json:"context"`     // "buying","bought","watching","warning","dev_wallet"
	Confidence float64 `json:"confidence"`
}

type WalletRef struct {
	Address    string `json:"address"`
	Chain      string `json:"chain"`
	OwnerType  string `json:"owner_type"`  // "kol_self","kol_wash","dev","contract","other_kol","unknown"
	Context    string `json:"context"`     // why this wallet was mentioned
	Confidence float64 `json:"confidence"`
}

func (e *Engine) AnalyzeSocialPost(ctx context.Context, kolName string, post db.SocialPost, recentContext []db.SocialPost) (*PostAnalysis, error) {
	if !e.IsEnabled() {
		return nil, fmt.Errorf("AI engine not enabled")
	}

	// Build context from recent posts
	var contextPosts string
	for _, p := range recentContext {
		contextPosts += fmt.Sprintf("[%s] %s\n---\n", p.PostedAt.Format("Jan 2 15:04"), truncate(p.Content, 200))
	}

	prompt := fmt.Sprintf(`You are an expert crypto on-chain analyst specializing in KOL (Key Opinion Leader) wallet detection.

Analyze this social media post from crypto KOL "%s":

POST:
%s

RECENT POSTS (for context):
%s

Analyze deeply and return a JSON object with these fields:
{
  "intent": "buy_call|sell_alert|shill|analysis|meme|warning|paid_promo",
  "sentiment": "extremely_bullish|bullish|neutral|bearish|fud",
  "tokens": [
    {"address": "if found", "symbol": "$TICKER", "chain": "solana|ethereum|base|bsc", "context": "buying|bought|watching|warning|dev_wallet", "confidence": 0.0-1.0}
  ],
  "wallets": [
    {"address": "addr", "chain": "solana|ethereum|base|bsc", "owner_type": "kol_self|kol_wash|dev|contract|other_kol|unknown", "context": "why mentioned", "confidence": 0.0-1.0}
  ],
  "trading_bots": ["bonkbot","trojan","maestro","photon","bullx","axiom","bloom","banana_gun"],
  "is_paid_promo": false,
  "urgency": "high|medium|low",
  "buy_before_post": false,
  "expected_action": "will_buy|already_bought|selling|observing",
  "risk_of_wash": "high|medium|low",
  "reasoning": "brief explanation of your analysis",
  "confidence_score": 0.0-1.0
}

KEY ANALYSIS POINTS:
- If the KOL says "ape", "loaded", "buying" = they're buying/have bought
- If they share a CA/dexscreener link = very likely they want followers to buy
- If they say "this dev's wallet" or "whale wallet" = NOT the KOL's wallet
- Look for signs of pre-buying: phrases like "got in early", "been watching", "accumulated"
- Paid promos often use: "🚀", "NFA", "not financial advice", "#ad", forced-sounding enthusiasm
- Look for trading bot references in screenshots or text: BonkBot, Trojan, Maestro, Photon, BullX, Axiom, Bloom
- High wash trading risk: new token + overly enthusiastic + no real analysis + "trust me" language

Return ONLY valid JSON, no other text.`, kolName, post.Content, contextPosts)

	resp, err := e.callLLM(ctx, prompt, "json")
	if err != nil {
		return nil, err
	}

	var analysis PostAnalysis
	if err := json.Unmarshal([]byte(resp), &analysis); err != nil {
		// Try to extract JSON from response
		if start := strings.Index(resp, "{"); start >= 0 {
			if end := strings.LastIndex(resp, "}"); end > start {
				json.Unmarshal([]byte(resp[start:end+1]), &analysis)
			}
		}
	}

	return &analysis, nil
}

// ============================================================================
// 2. WALLET RELATIONSHIP ANALYSIS
// ============================================================================

// AnalyzeWalletRelationship asks the LLM to determine if two wallets are likely
// controlled by the same person, based on behavioral patterns.
type WalletRelationship struct {
	Wallet1           string  `json:"wallet_1"`
	Wallet2           string  `json:"wallet_2"`
	SameOwnerProb     float64 `json:"same_owner_probability"` // 0.0-1.0
	RelationshipType  string  `json:"relationship_type"`      // "same_owner","wash_wallet","linked","unrelated"
	Evidence          []string `json:"evidence"`
	Reasoning         string  `json:"reasoning"`
}

func (e *Engine) AnalyzeWalletRelationship(ctx context.Context, kolName string, wallet1, wallet2 WalletProfile) (*WalletRelationship, error) {
	if !e.IsEnabled() {
		return nil, fmt.Errorf("AI engine not enabled")
	}

	prompt := fmt.Sprintf(`You are an expert blockchain forensic analyst. Determine if these two wallets are controlled by the same person (crypto KOL: "%s").

WALLET 1 (%s - %s):
- First active: %s
- Total transactions: %d
- Buy amounts (USD): min=$%.0f, max=$%.0f, avg=$%.0f
- Preferred DEX: %s
- Priority fee avg: %.0f
- Active hours (UTC): %s
- Tokens traded: %s
- Funding source: %s
- Funding amounts: %s
- ENS/SNS names: %s

WALLET 2 (%s - %s):
- First active: %s
- Total transactions: %d
- Buy amounts (USD): min=$%.0f, max=$%.0f, avg=$%.0f
- Preferred DEX: %s
- Priority fee avg: %.0f
- Active hours (UTC): %s
- Tokens traded: %s
- Funding source: %s
- Funding amounts: %s
- ENS/SNS names: %s

KNOWN LINKS:
- Direct transfers between them: %v
- Shared token trades: %d tokens in common
- Trades within 5 min of each other: %d times
- Funding amount match (within 3%%): %v

Return JSON:
{
  "same_owner_probability": 0.0-1.0,
  "relationship_type": "same_owner|wash_wallet|linked|funded_by_same|unrelated",
  "evidence": ["point 1","point 2",...],
  "reasoning": "detailed explanation"
}

KEY INDICATORS OF SAME OWNER:
- Same priority fee settings (bot configuration is unique)
- Same DEX preferences
- Similar buy amounts
- Trading same tokens at similar times
- One wallet was funded shortly after other sent funds out
- Both active during same hours
- Amount sent from wallet1 matches amount received by wallet2 minus ~1-2%% (FixedFloat fee)
- Fresh wallet (< 7 days) that immediately starts trading KOL-mentioned tokens

Return ONLY valid JSON.`,
		kolName,
		wallet1.Address, wallet1.Chain, wallet1.FirstActive, wallet1.TxCount,
		wallet1.MinBuy, wallet1.MaxBuy, wallet1.AvgBuy,
		wallet1.TopDEX, wallet1.AvgFee, wallet1.ActiveHours, wallet1.TopTokens,
		wallet1.FundingSource, wallet1.FundingAmounts, wallet1.Names,
		wallet2.Address, wallet2.Chain, wallet2.FirstActive, wallet2.TxCount,
		wallet2.MinBuy, wallet2.MaxBuy, wallet2.AvgBuy,
		wallet2.TopDEX, wallet2.AvgFee, wallet2.ActiveHours, wallet2.TopTokens,
		wallet2.FundingSource, wallet2.FundingAmounts, wallet2.Names,
		wallet1.DirectTransfersTo(wallet2.Address), wallet1.SharedTokenCount(wallet2), wallet1.NearSimultaneousTrades(wallet2), wallet1.AmountMatchesWith(wallet2),
	)

	resp, err := e.callLLM(ctx, prompt, "json")
	if err != nil {
		return nil, err
	}

	var rel WalletRelationship
	json.Unmarshal(extractJSON(resp), &rel)
	rel.Wallet1 = wallet1.Address
	rel.Wallet2 = wallet2.Address
	return &rel, nil
}

// WalletProfile is a summary of a wallet's behavior for LLM analysis.
type WalletProfile struct {
	Address        string
	Chain          string
	FirstActive    string
	TxCount        int
	MinBuy         float64
	MaxBuy         float64
	AvgBuy         float64
	TopDEX         string
	AvgFee         float64
	ActiveHours    string
	TopTokens      string
	FundingSource  string
	FundingAmounts string
	Names          string // ENS/SNS

	// For comparison methods
	Tokens        []string
	TradeTimestamps []time.Time
	OutgoingAmounts []float64
	IncomingAmounts []float64
	TransferDests   []string
}

func (wp WalletProfile) DirectTransfersTo(addr string) bool {
	for _, d := range wp.TransferDests {
		if strings.EqualFold(d, addr) {
			return true
		}
	}
	return false
}

func (wp WalletProfile) SharedTokenCount(other WalletProfile) int {
	set := map[string]bool{}
	for _, t := range wp.Tokens {
		set[t] = true
	}
	count := 0
	for _, t := range other.Tokens {
		if set[t] {
			count++
		}
	}
	return count
}

func (wp WalletProfile) NearSimultaneousTrades(other WalletProfile) int {
	count := 0
	for _, t1 := range wp.TradeTimestamps {
		for _, t2 := range other.TradeTimestamps {
			diff := t1.Sub(t2)
			if diff < 0 {
				diff = -diff
			}
			if diff < 5*time.Minute {
				count++
			}
		}
	}
	return count
}

func (wp WalletProfile) AmountMatchesWith(other WalletProfile) bool {
	for _, out := range wp.OutgoingAmounts {
		for _, in := range other.IncomingAmounts {
			if out > 0 && in > 0 {
				diff := (out - in) / out
				if diff > 0 && diff < 0.03 { // within 3% (FixedFloat range)
					return true
				}
			}
		}
	}
	return false
}

// BuildWalletProfile constructs a WalletProfile from DB data for LLM analysis.
func (e *Engine) BuildWalletProfile(walletID int64, address string, chain config.Chain) WalletProfile {
	wp := WalletProfile{
		Address: address,
		Chain:   string(chain),
	}

	txs, _ := e.store.GetTransactionsForWallet(walletID, 500)
	if len(txs) == 0 {
		return wp
	}

	wp.TxCount = len(txs)

	dexCounts := map[string]int{}
	hourCounts := [24]int{}
	tokenSet := map[string]bool{}
	var buyAmounts, fees []float64

	for _, tx := range txs {
		if !tx.Timestamp.IsZero() {
			hourCounts[tx.Timestamp.UTC().Hour()]++
			wp.TradeTimestamps = append(wp.TradeTimestamps, tx.Timestamp)
			if wp.FirstActive == "" {
				wp.FirstActive = tx.Timestamp.Format("2006-01-02")
			}
		}
		if tx.Platform != "" {
			dexCounts[tx.Platform]++
		}
		if tx.TokenAddress != "" {
			tokenSet[tx.TokenAddress] = true
			wp.Tokens = append(wp.Tokens, tx.TokenAddress)
		}
		if tx.TxType == "swap_buy" && tx.AmountUSD > 0 {
			buyAmounts = append(buyAmounts, tx.AmountUSD)
		}
		if tx.PriorityFee > 0 {
			fees = append(fees, tx.PriorityFee)
		}
		if tx.TxType == "transfer_out" {
			wp.OutgoingAmounts = append(wp.OutgoingAmounts, tx.AmountToken)
			wp.TransferDests = append(wp.TransferDests, tx.ToAddress)
		}
		if tx.TxType == "transfer_in" {
			wp.IncomingAmounts = append(wp.IncomingAmounts, tx.AmountToken)
		}
	}

	if len(buyAmounts) > 0 {
		wp.MinBuy = min(buyAmounts)
		wp.MaxBuy = max(buyAmounts)
		wp.AvgBuy = avg(buyAmounts)
	}
	if len(fees) > 0 {
		wp.AvgFee = avg(fees)
	}

	// Top DEX
	topDEX, topCount := "", 0
	for d, c := range dexCounts {
		if c > topCount {
			topDEX, topCount = d, c
		}
	}
	wp.TopDEX = topDEX

	// Active hours
	var activeHours []string
	for h, c := range hourCounts {
		if c > len(txs)/24 { // above average
			activeHours = append(activeHours, fmt.Sprintf("%d:00", h))
		}
	}
	wp.ActiveHours = strings.Join(activeHours, ",")

	// Top tokens (first 10)
	i := 0
	var topTokens []string
	for t := range tokenSet {
		if i >= 10 {
			break
		}
		topTokens = append(topTokens, abbrev(t))
		i++
	}
	wp.TopTokens = strings.Join(topTokens, ",")

	return wp
}

// ============================================================================
// 3. INTELLIGENT NEW WALLET DISCOVERY
// ============================================================================

// DiscoverNewWallets analyzes a KOL's recent social activity + on-chain data
// and asks the LLM to identify wallets that are likely controlled by the KOL
// but not yet in our database.
type WalletDiscovery struct {
	SuggestedWallets []SuggestedWallet `json:"suggested_wallets"`
	Reasoning        string            `json:"reasoning"`
	AnalysisNotes    string            `json:"analysis_notes"`
}

type SuggestedWallet struct {
	Address    string  `json:"address"`
	Chain      string  `json:"chain"`
	OwnerType  string  `json:"owner_type"`  // "main","trading","wash","sniping"
	Evidence   string  `json:"evidence"`
	Confidence float64 `json:"confidence"`
	Priority   string  `json:"priority"`    // "investigate_now","monitor","low_priority"
}

func (e *Engine) DiscoverNewWallets(ctx context.Context, kolID int64) (*WalletDiscovery, error) {
	if !e.IsEnabled() {
		return nil, fmt.Errorf("AI engine not enabled")
	}

	kol, _ := e.store.GetKOLByID(kolID)
	if kol == nil {
		return nil, fmt.Errorf("KOL not found")
	}

	// Gather all data
	knownWallets, _ := e.store.GetWalletsForKOL(kolID)
	recentPosts, _ := e.store.GetPostsForKOL(kolID, 50)
	recentMentions, _ := e.store.GetRecentTokenMentions(168)
	washCandidates, _ := e.store.GetWashCandidatesForKOL(kolID)

	// Build summary strings
	var walletSummary strings.Builder
	for _, w := range knownWallets {
		walletSummary.WriteString(fmt.Sprintf("  - %s (%s, %s, confidence: %.0f%%, source: %s)\n",
			abbrev(w.Address), w.Chain, w.Label, w.Confidence*100, w.Source))
	}

	var postSummary strings.Builder
	for _, p := range recentPosts {
		postSummary.WriteString(fmt.Sprintf("[%s] %s\n", p.PostedAt.Format("Jan 2 15:04"), truncate(p.Content, 150)))
	}

	var mentionSummary strings.Builder
	kolMentions := 0
	for _, m := range recentMentions {
		if m.KOLID == kolID {
			mentionSummary.WriteString(fmt.Sprintf("  - %s (%s) at %s\n", m.TokenSymbol, abbrev(m.TokenAddress), m.MentionedAt.Format("Jan 2 15:04")))
			kolMentions++
		}
	}

	var washSummary strings.Builder
	for _, c := range washCandidates {
		washSummary.WriteString(fmt.Sprintf("  - %s (%s, score: %.0f%%, funding: %s, bought_same_tokens: %v)\n",
			abbrev(c.Address), c.Chain, c.ConfidenceScore*100, c.FundingSourceType, c.BoughtSameToken))
	}

	prompt := fmt.Sprintf(`You are an expert blockchain forensic analyst. Analyze this KOL's data and suggest wallets that might belong to them but aren't tracked yet.

KOL: %s (@%s, TG: %s)

KNOWN WALLETS:
%s

RECENT SOCIAL POSTS (last 7 days):
%s

TOKEN MENTIONS (%d total):
%s

CURRENT WASH WALLET SUSPECTS:
%s

YOUR TASK:
1. Analyze the KOL's posting patterns - are there tokens they seem to know a lot about before posting?
2. Look for wallet addresses mentioned in posts that we should track
3. Based on the wash wallet suspects, which ones should we prioritize investigating?
4. Are there patterns suggesting wallets we haven't found yet?

IMPORTANT: Consider these KOL wash wallet techniques:
- Using FixedFloat to obscure fund origin (sends SOL/ETH → receives on fresh wallet minus 1-2%% fee)
- Creating new wallets for each token to avoid pattern detection
- Using different trading bots on wash vs main wallet
- Buying seconds before posting (pre-buy detection)
- Using bridges to fund wallets on different chains
- Having a "sniper" wallet that buys immediately when liquidity appears, separate from main

Return JSON:
{
  "suggested_wallets": [
    {
      "address": "if known, or describe what to look for",
      "chain": "solana|ethereum|base|bsc",
      "owner_type": "main|trading|wash|sniping|funding",
      "evidence": "why you think this wallet exists or should be investigated",
      "confidence": 0.0-1.0,
      "priority": "investigate_now|monitor|low_priority"
    }
  ],
  "reasoning": "your overall analysis of this KOL's wallet infrastructure",
  "analysis_notes": "recommendations for the tracker (e.g., monitor specific tokens, check bridges, etc.)"
}

Return ONLY valid JSON.`,
		kol.Name, kol.TwitterHandle, kol.TelegramChannel,
		walletSummary.String(),
		postSummary.String(),
		kolMentions, mentionSummary.String(),
		washSummary.String(),
	)

	resp, err := e.callLLM(ctx, prompt, "json")
	if err != nil {
		return nil, err
	}

	var discovery WalletDiscovery
	json.Unmarshal(extractJSON(resp), &discovery)
	return &discovery, nil
}

// ============================================================================
// 4. PERIODIC AI ANALYSIS (runs on schedule)
// ============================================================================

// RunPeriodicAnalysis is called by the main loop to run AI analysis on all KOLs.
func (e *Engine) RunPeriodicAnalysis(ctx context.Context) error {
	if !e.IsEnabled() {
		return nil
	}

	kols, _ := e.store.GetKOLs()
	for _, kol := range kols {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 1. Analyze unprocessed posts
		posts, _ := e.store.GetUnprocessedPosts()
		for _, post := range posts {
			if post.KOLID != kol.ID {
				continue
			}

			recentPosts, _ := e.store.GetPostsForKOL(kol.ID, 10)
			analysis, err := e.AnalyzeSocialPost(ctx, kol.Name, post, recentPosts)
			if err != nil {
				log.Debug().Err(err).Msg("AI post analysis failed")
				continue
			}

			// Process discovered wallets
			for _, wr := range analysis.Wallets {
				if wr.OwnerType == "kol_self" || wr.OwnerType == "kol_wash" {
					chain := config.Chain(wr.Chain)
					if chain == "" {
						chain = config.ChainSolana
					}
					conf := wr.Confidence
					if wr.OwnerType == "kol_wash" {
						conf *= 0.7 // lower confidence for suspected wash
					}
					e.store.UpsertWallet(kol.ID, wr.Address, chain,
						"ai:"+wr.OwnerType, conf,
						fmt.Sprintf("ai_analysis:post:%s", post.PostID))

					log.Info().
						Str("address", abbrev(wr.Address)).
						Str("type", wr.OwnerType).
						Float64("confidence", conf).
						Msg("🤖 AI discovered wallet from post")
				}
			}

			// Store token refs with AI context
			for _, tr := range analysis.Tokens {
				chain := config.Chain(tr.Chain)
				if chain == "" {
					chain = config.ChainSolana
				}
				if tr.Address != "" {
					e.store.InsertTokenMention(kol.ID, post.ID, tr.Address, tr.Symbol, chain, post.PostedAt)
				}
			}

			// Generate alerts for high-risk findings
			if analysis.RiskOfWashTrading == "high" {
				e.store.InsertAlert(kol.ID, "ai_wash_risk", "warning",
					fmt.Sprintf("AI: High wash trading risk in post from %s", kol.Name),
					analysis.Reasoning, "", "")
			}
			if analysis.BuyBeforePost {
				e.store.InsertAlert(kol.ID, "ai_pre_buy", "critical",
					fmt.Sprintf("AI: Pre-buy detected for %s", kol.Name),
					analysis.Reasoning, "", "")
			}

			e.store.MarkPostProcessed(post.ID)

			// Rate limit between posts
			time.Sleep(2 * time.Second)
		}

		// 2. Run wallet discovery periodically (expensive, run less often)
		discovery, err := e.DiscoverNewWallets(ctx, kol.ID)
		if err == nil && discovery != nil {
			for _, sw := range discovery.SuggestedWallets {
				if sw.Address != "" && sw.Confidence >= 0.5 {
					chain := config.Chain(sw.Chain)
					if chain == "" {
						chain = config.ChainSolana
					}
					e.store.UpsertWallet(kol.ID, sw.Address, chain,
						"ai:"+sw.OwnerType, sw.Confidence,
						"ai_discovery")

					if sw.Priority == "investigate_now" {
						e.store.InsertAlert(kol.ID, "ai_wallet_found", "warning",
							fmt.Sprintf("AI: New wallet suggested for %s (%s)", kol.Name, sw.OwnerType),
							sw.Evidence, sw.Address, "")
					}

					log.Info().
						Str("address", abbrev(sw.Address)).
						Str("type", sw.OwnerType).
						Float64("confidence", sw.Confidence).
						Msg("🤖 AI suggested new wallet to track")
				}
			}
		}

		time.Sleep(5 * time.Second) // rate limit between KOLs
	}

	return nil
}

// ============================================================================
// 5. UPDATE EXISTING WALLET CLASSIFICATIONS
// ============================================================================

// ReclassifyWallets uses the LLM to re-evaluate wallet labels and confidence
// based on accumulated data. A wallet originally tagged "from_tweet" might
// now have enough on-chain evidence to be upgraded to "confirmed_main" or
// downgraded to "probably_not_kol".
func (e *Engine) ReclassifyWallets(ctx context.Context, kolID int64) ([]WalletUpdate, error) {
	if !e.IsEnabled() {
		return nil, nil
	}

	wallets, _ := e.store.GetWalletsForKOL(kolID)
	kol, _ := e.store.GetKOLByID(kolID)
	if kol == nil || len(wallets) == 0 {
		return nil, nil
	}

	// Build profiles for all wallets
	var profileSummaries strings.Builder
	for _, w := range wallets {
		txs, _ := e.store.GetTransactionsForWallet(w.ID, 100)
		profileSummaries.WriteString(fmt.Sprintf(`
WALLET: %s
  Chain: %s | Label: %s | Confidence: %.0f%% | Source: %s
  Transactions: %d
`, w.Address, w.Chain, w.Label, w.Confidence*100, w.Source, len(txs)))

		if len(txs) > 0 {
			var buyCount, sellCount int
			for _, tx := range txs {
				if tx.TxType == "swap_buy" { buyCount++ }
				if tx.TxType == "swap_sell" { sellCount++ }
			}
			profileSummaries.WriteString(fmt.Sprintf("  Buys: %d, Sells: %d\n", buyCount, sellCount))
		}
	}

	prompt := fmt.Sprintf(`You are a blockchain forensic analyst. Review these wallets attributed to KOL "%s" and update their classifications.

%s

For each wallet, assess:
1. Is this really the KOL's wallet? (confidence update)
2. What type of wallet is it? (main, trading, wash, sniping, not_kol)
3. Should we keep tracking it?

Return JSON:
{
  "updates": [
    {
      "address": "wallet_address",
      "new_label": "main|trading|wash|sniping|not_kol|unknown",
      "new_confidence": 0.0-1.0,
      "reasoning": "why this classification",
      "keep_tracking": true
    }
  ]
}

Return ONLY valid JSON.`, kol.Name, profileSummaries.String())

	resp, err := e.callLLM(ctx, prompt, "json")
	if err != nil {
		return nil, err
	}

	var result struct {
		Updates []WalletUpdate `json:"updates"`
	}
	json.Unmarshal(extractJSON(resp), &result)

	// Apply updates
	for _, u := range result.Updates {
		for _, w := range wallets {
			if w.Address == u.Address {
				e.store.UpsertWallet(kolID, u.Address, w.Chain,
					"ai:"+u.NewLabel, u.NewConfidence,
					"ai_reclassify")
				log.Info().
					Str("wallet", abbrev(u.Address)).
					Str("old", w.Label).
					Str("new", u.NewLabel).
					Float64("conf", u.NewConfidence).
					Msg("🤖 AI reclassified wallet")
				break
			}
		}
	}

	return result.Updates, nil
}

type WalletUpdate struct {
	Address       string  `json:"address"`
	NewLabel      string  `json:"new_label"`
	NewConfidence float64 `json:"new_confidence"`
	Reasoning     string  `json:"reasoning"`
	KeepTracking  bool    `json:"keep_tracking"`
}

// ============================================================================
// LLM CALL ABSTRACTION (supports Anthropic, OpenAI, Ollama)
// ============================================================================

func (e *Engine) callLLM(ctx context.Context, prompt, format string) (string, error) {
	switch e.provider {
	case "anthropic":
		return e.callAnthropic(ctx, prompt)
	case "openai":
		return e.callOpenAI(ctx, prompt)
	case "ollama":
		return e.callOllama(ctx, prompt)
	default:
		return "", fmt.Errorf("no AI provider configured")
	}
}

func (e *Engine) callAnthropic(ctx context.Context, prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model":      e.model,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", e.apiBaseURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", e.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	json.Unmarshal(respBody, &result)

	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}
	return "", fmt.Errorf("empty response from anthropic")
}

func (e *Engine) callOpenAI(ctx context.Context, prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model": e.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 4096,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", e.apiBaseURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(respBody, &result)

	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("empty response from openai")
}

func (e *Engine) callOllama(ctx context.Context, prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model": e.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream": false,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", e.apiBaseURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	json.Unmarshal(respBody, &result)
	return result.Message.Content, nil
}

// ============================================================================
// HELPERS
// ============================================================================

func extractJSON(s string) []byte {
	s = strings.TrimSpace(s)
	// Remove markdown code fences
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	// Find JSON boundaries
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return []byte(s[start : end+1])
	}
	return []byte(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func abbrev(a string) string {
	if len(a) > 12 {
		return a[:6] + "..." + a[len(a)-4:]
	}
	return a
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range vals {
		s += v
	}
	return s / float64(len(vals))
}

func min(vals []float64) float64 {
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func max(vals []float64) float64 {
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func envOr(key, fallback string) string {
	if v := fmt.Sprintf("%s", key); v != "" {
		// This is a placeholder - actual env reading is in config
		return fallback
	}
	return fallback
}
