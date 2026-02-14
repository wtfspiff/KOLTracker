package db

import (
	"time"

	"github.com/kol-tracker/pkg/config"
)

// ---- Core Models ----

type KOLProfile struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	TwitterHandle   string    `json:"twitter_handle"`
	TelegramChannel string    `json:"telegram_channel"`
	Notes           string    `json:"notes"`
	CreatedAt       time.Time `json:"created_at"`
}

type TrackedWallet struct {
	ID           int64        `json:"id"`
	KOLID        int64        `json:"kol_id"`
	Address      string       `json:"address"`
	Chain        config.Chain `json:"chain"`
	Label        string       `json:"label"`        // "main","trading","wash_suspected","from_tweet"
	Confidence   float64      `json:"confidence"`    // 1.0=confirmed, 0-0.99=suspected
	Source       string       `json:"source"`        // "manual","tweet:id","telegram:ch:id","on-chain-link","pattern-match"
	DiscoveredAt time.Time    `json:"discovered_at"`
	Metadata     string       `json:"metadata"`      // JSON
}

type SocialPost struct {
	ID               int64     `json:"id"`
	KOLID            int64     `json:"kol_id"`
	Platform         string    `json:"platform"` // "twitter","telegram"
	PostID           string    `json:"post_id"`
	Content          string    `json:"content"`
	PostedAt         time.Time `json:"posted_at"`
	ExtractedTokens  string    `json:"extracted_tokens"`  // JSON array
	ExtractedWallets string    `json:"extracted_wallets"` // JSON array
	ExtractedLinks   string    `json:"extracted_links"`   // JSON array
	Processed        bool      `json:"processed"`
	CreatedAt        time.Time `json:"created_at"`
}

type TokenMention struct {
	ID            int64        `json:"id"`
	KOLID         int64        `json:"kol_id"`
	PostID        int64        `json:"post_id"`
	TokenAddress  string       `json:"token_address"`
	TokenSymbol   string       `json:"token_symbol"`
	Chain         config.Chain `json:"chain"`
	MentionedAt   time.Time    `json:"mentioned_at"`
	PreBuyDetected  bool       `json:"pre_buy_detected"`
	PostBuyDetected bool       `json:"post_buy_detected"`
}

type WalletTransaction struct {
	ID            int64        `json:"id"`
	WalletID      int64        `json:"wallet_id"`
	TxHash        string       `json:"tx_hash"`
	Chain         config.Chain `json:"chain"`
	TxType        string       `json:"tx_type"` // "swap_buy","swap_sell","transfer_in","transfer_out","bridge_in"
	TokenAddress  string       `json:"token_address"`
	TokenSymbol   string       `json:"token_symbol"`
	AmountToken   float64      `json:"amount_token"`
	AmountUSD     float64      `json:"amount_usd"`
	FromAddress   string       `json:"from_address"`
	ToAddress     string       `json:"to_address"`
	Timestamp     time.Time    `json:"timestamp"`
	BlockNumber   int64        `json:"block_number"`
	Platform      string       `json:"platform"` // DEX name: "raydium","jupiter","uniswap","pancakeswap"
	PriorityFee   float64      `json:"priority_fee"`
	Metadata      string       `json:"metadata"` // JSON
}

type WashWalletCandidate struct {
	ID                int64        `json:"id"`
	Address           string       `json:"address"`
	Chain             config.Chain `json:"chain"`
	FundedBy          string       `json:"funded_by"`
	FundingSourceType string       `json:"funding_source_type"` // "fixedfloat","bridge","mixer","direct","cex","unknown"
	FundingAmount     float64      `json:"funding_amount"`
	FundingToken      string       `json:"funding_token"`
	FundingTx         string       `json:"funding_tx"`
	FirstSeen         time.Time    `json:"first_seen"`

	// Match signals
	BoughtSameToken    bool    `json:"bought_same_token"`
	TimingMatch        bool    `json:"timing_match"`
	AmountPatternMatch bool    `json:"amount_pattern_match"`
	BotSignatureMatch  bool    `json:"bot_signature_match"`
	ConfidenceScore    float64 `json:"confidence_score"`

	LinkedKOLID int64  `json:"linked_kol_id"`
	Status      string `json:"status"` // "candidate","confirmed","dismissed"
	Notes       string `json:"notes"`
	CreatedAt   time.Time `json:"created_at"`
}

type TradingPattern struct {
	ID          int64     `json:"id"`
	KOLID       int64     `json:"kol_id"`
	PatternType string    `json:"pattern_type"`
	PatternData string    `json:"pattern_data"` // JSON
	SampleCount int       `json:"sample_count"`
	LastUpdated time.Time `json:"last_updated"`
}

type FundingFlowMatch struct {
	ID              int64        `json:"id"`
	SourceTx        string       `json:"source_tx"`
	SourceChain     config.Chain `json:"source_chain"`
	SourceAmount    float64      `json:"source_amount"`
	SourceToken     string       `json:"source_token"`
	DestAddress     string       `json:"dest_address"`
	DestChain       config.Chain `json:"dest_chain"`
	DestAmount      float64      `json:"dest_amount"`
	DestToken       string       `json:"dest_token"`
	Service         string       `json:"service"`
	AmountDiffPct   float64      `json:"amount_diff_pct"`
	TimeDiffSeconds int64        `json:"time_diff_seconds"`
	MatchConfidence float64      `json:"match_confidence"`
	CreatedAt       time.Time    `json:"created_at"`
}

type Alert struct {
	ID            int64     `json:"id"`
	KOLID         int64     `json:"kol_id"`
	AlertType     string    `json:"alert_type"`
	Severity      string    `json:"severity"` // "info","warning","critical"
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	RelatedWallet string    `json:"related_wallet"`
	RelatedToken  string    `json:"related_token"`
	Metadata      string    `json:"metadata"`
	CreatedAt     time.Time `json:"created_at"`
}

// ---- Extraction Result (from social media parsing) ----

type ExtractionResult struct {
	SolanaAddresses []string `json:"solana_addresses"`
	EVMAddresses    []string `json:"evm_addresses"`
	TokenSymbols    []string `json:"token_symbols"`    // $TICKER mentions
	ContractAddrs   []string `json:"contract_addrs"`   // Explicit CAs in text
	TokenCAsFromLinks []string `json:"token_cas_from_links"` // CAs extracted from dex links

	DexScreenerLinks []string `json:"dexscreener_links"`
	BirdeyeLinks     []string `json:"birdeye_links"`
	PumpFunLinks     []string `json:"pump_fun_links"`
	PhotonLinks      []string `json:"photon_links"`
	GmgnLinks        []string `json:"gmgn_links"`
	BullxLinks       []string `json:"bullx_links"`
	OtherLinks       []string `json:"other_links"`

	BotSignals map[string]bool `json:"bot_signals"` // detected trading bot mentions
	BuySignal  bool            `json:"buy_signal"`
	SellSignal bool            `json:"sell_signal"`
}

func (e *ExtractionResult) AllTokenCAs() []string {
	seen := map[string]bool{}
	var result []string
	for _, lists := range [][]string{e.ContractAddrs, e.TokenCAsFromLinks} {
		for _, ca := range lists {
			if !seen[ca] {
				seen[ca] = true
				result = append(result, ca)
			}
		}
	}
	return result
}

func (e *ExtractionResult) AllAddresses() []string {
	var result []string
	result = append(result, e.SolanaAddresses...)
	result = append(result, e.EVMAddresses...)
	return result
}

func (e *ExtractionResult) HasContent() bool {
	return len(e.SolanaAddresses) > 0 || len(e.EVMAddresses) > 0 ||
		len(e.TokenSymbols) > 0 || len(e.AllTokenCAs()) > 0
}

// ---- Funding Analysis Result ----

type FundingSource struct {
	SourceAddress string       `json:"source_address"`
	Amount        float64      `json:"amount"`
	Token         string       `json:"token"`
	TxHash        string       `json:"tx_hash"`
	SourceType    string       `json:"source_type"` // "fixedfloat","bridge","mixer","cex","unknown"
	Timestamp     int64        `json:"timestamp"`
	Chain         config.Chain `json:"chain"`
}

type FundingAnalysis struct {
	Address        string          `json:"address"`
	Chain          config.Chain    `json:"chain"`
	FundingSources []FundingSource `json:"funding_sources"`
	IsNewWallet    bool            `json:"is_new_wallet"`
	FirstTxTime    *time.Time      `json:"first_tx_time"`
	TotalFunded    float64         `json:"total_funded"`
	NativeSymbol   string          `json:"native_symbol"` // SOL, ETH, BNB
}

// ---- Wash Wallet Score Breakdown ----

type WashScore struct {
	Address      string       `json:"address"`
	Chain        config.Chain `json:"chain"`
	TotalScore   float64      `json:"total_score"`
	Signals      map[string]interface{} `json:"signals"`
}
