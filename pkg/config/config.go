package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Chain string

const (
	ChainSolana   Chain = "solana"
	ChainEthereum Chain = "ethereum"
	ChainBase     Chain = "base"
	ChainBSC      Chain = "bsc"
)

func AllEVMChains() []Chain {
	return []Chain{ChainEthereum, ChainBase, ChainBSC}
}

func AllChains() []Chain {
	return []Chain{ChainSolana, ChainEthereum, ChainBase, ChainBSC}
}

type KnownWallet struct {
	Address string
	Chain   Chain
	Label   string
}

type Config struct {
	// Twitter
	TwitterBearerToken string
	NitterInstances    []string
	TwitterPollInterval time.Duration
	// Twitter private API (imperatrona/twitter-scraper)
	TwitterUsername    string
	TwitterPassword   string
	TwitterEmail      string
	TwitterAuthToken  string // auth_token cookie
	TwitterCSRFToken  string // ct0 cookie
	TwitterCookieFile string // persist sessions

	// Telegram
	TelegramAPIID   int
	TelegramAPIHash string
	TelegramPhone   string
	TelegramPollInterval time.Duration

	// Solana
	SolanaRPCURL  string
	SolanaWSURL   string
	HeliusAPIKey  string
	HeliusRPCURL  string
	SolscanAPIKey string

	// EVM RPCs
	EVMRPC map[Chain]string

	// Block Explorer API keys
	ExplorerKeys map[Chain]string

	// Price APIs
	BirdeyeAPIKey   string
	DexScreenerAPI  string

	// KOL targets
	KOLTwitterHandles  []string
	KOLTelegramChannels []string
	KOLKnownWallets    []KnownWallet

	// Intervals
	ChainScanInterval       time.Duration
	PatternAnalysisInterval time.Duration
	FreshBuyerScanInterval  time.Duration

	// Detection thresholds
	WashWalletMinScore      float64
	AmountMatchTolerancePct float64
	FreshWalletAgeHours     int
	PreBuyWindowSeconds     int
	PostBuyWindowSeconds    int

	// DB
	DBPath string

	// Dashboard
	DashboardPort int

	// AI / LLM
	AnthropicAPIKey string
	OpenAIAPIKey    string
	OllamaURL       string
	AIModel         string
	AIAnalysisInterval time.Duration
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		TwitterBearerToken: os.Getenv("TWITTER_BEARER_TOKEN"),
		TwitterUsername:    os.Getenv("TWITTER_USERNAME"),
		TwitterPassword:    os.Getenv("TWITTER_PASSWORD"),
		TwitterEmail:       os.Getenv("TWITTER_EMAIL"),
		TwitterAuthToken:   os.Getenv("TWITTER_AUTH_TOKEN"),
		TwitterCSRFToken:   os.Getenv("TWITTER_CSRF_TOKEN"),
		TwitterCookieFile:  envOr("TWITTER_COOKIE_FILE", "twitter_cookies.json"),
		TelegramAPIHash:    os.Getenv("TELEGRAM_API_HASH"),
		TelegramPhone:      os.Getenv("TELEGRAM_PHONE"),

		SolanaRPCURL:  envOr("SOLANA_RPC_URL", "https://api.mainnet-beta.solana.com"),
		SolanaWSURL:   envOr("SOLANA_WS_URL", "wss://api.mainnet-beta.solana.com"),
		HeliusAPIKey:  os.Getenv("HELIUS_API_KEY"),
		HeliusRPCURL:  os.Getenv("HELIUS_RPC_URL"),
		SolscanAPIKey: os.Getenv("SOLSCAN_API_KEY"),

		BirdeyeAPIKey:  os.Getenv("BIRDEYE_API_KEY"),
		DexScreenerAPI: envOr("DEXSCREENER_API", "https://api.dexscreener.com"),

		DBPath:        envOr("DB_PATH", "kol_tracker.db"),
		DashboardPort: envInt("DASHBOARD_PORT", 8080),

		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		OllamaURL:       os.Getenv("OLLAMA_URL"),
		AIModel:         envOr("AI_MODEL", "claude-sonnet-4-20250514"),
		AIAnalysisInterval: time.Duration(envInt("AI_ANALYSIS_INTERVAL", 600)) * time.Second,

		WashWalletMinScore:      envFloat("WASH_WALLET_MIN_SCORE", 0.4),
		AmountMatchTolerancePct: envFloat("AMOUNT_MATCH_TOLERANCE_PCT", 3.0),
		FreshWalletAgeHours:     envInt("FRESH_WALLET_AGE_HOURS", 168),
		PreBuyWindowSeconds:     envInt("PRE_BUY_WINDOW_SECONDS", 3600),
		PostBuyWindowSeconds:    envInt("POST_BUY_WINDOW_SECONDS", 7200),

		TwitterPollInterval:     time.Duration(envInt("TWITTER_POLL_INTERVAL", 60)) * time.Second,
		TelegramPollInterval:    time.Duration(envInt("TELEGRAM_POLL_INTERVAL", 30)) * time.Second,
		ChainScanInterval:       time.Duration(envInt("CHAIN_SCAN_INTERVAL", 120)) * time.Second,
		PatternAnalysisInterval: time.Duration(envInt("PATTERN_ANALYSIS_INTERVAL", 300)) * time.Second,
		FreshBuyerScanInterval:  time.Duration(envInt("FRESH_BUYER_SCAN_INTERVAL", 15)) * time.Second,
	}

	// Telegram API ID
	if v := os.Getenv("TELEGRAM_API_ID"); v != "" {
		id, err := strconv.Atoi(v)
		if err == nil {
			cfg.TelegramAPIID = id
		}
	}

	// Nitter instances
	if v := os.Getenv("NITTER_INSTANCES"); v != "" {
		cfg.NitterInstances = splitTrim(v)
	} else {
		cfg.NitterInstances = []string{
			"https://nitter.privacydev.net",
		}
	}

	// EVM RPCs
	cfg.EVMRPC = map[Chain]string{
		ChainEthereum: envOr("ETH_RPC_URL", "https://eth.llamarpc.com"),
		ChainBase:     envOr("BASE_RPC_URL", "https://mainnet.base.org"),
		ChainBSC:      envOr("BSC_RPC_URL", "https://bsc-dataseed.binance.org"),
	}

	// Explorer keys
	cfg.ExplorerKeys = map[Chain]string{
		ChainEthereum: os.Getenv("ETHERSCAN_API_KEY"),
		ChainBase:     os.Getenv("BASESCAN_API_KEY"),
		ChainBSC:      os.Getenv("BSCSCAN_API_KEY"),
	}

	// KOL targets
	cfg.KOLTwitterHandles = splitTrim(os.Getenv("KOL_TWITTER_HANDLES"))
	cfg.KOLTelegramChannels = splitTrim(os.Getenv("KOL_TELEGRAM_CHANNELS"))

	// Parse known wallets: "addr:chain:label,addr:chain:label"
	for _, w := range splitTrim(os.Getenv("KOL_KNOWN_WALLETS")) {
		parts := strings.SplitN(w, ":", 3)
		if len(parts) >= 2 {
			kw := KnownWallet{
				Address: parts[0],
				Chain:   Chain(parts[1]),
			}
			if len(parts) == 3 {
				kw.Label = parts[2]
			}
			cfg.KOLKnownWallets = append(cfg.KOLKnownWallets, kw)
		}
	}

	return cfg, nil
}

func (c *Config) GetExplorerURL(chain Chain) string {
	switch chain {
	case ChainEthereum:
		return "https://api.etherscan.io/api"
	case ChainBase:
		return "https://api.basescan.org/api"
	case ChainBSC:
		return "https://api.bscscan.com/api"
	default:
		return ""
	}
}

func (c *Config) GetExplorerKey(chain Chain) string {
	return c.ExplorerKeys[chain]
}

func (c *Config) Validate() error {
	if len(c.KOLTwitterHandles) == 0 && len(c.KOLTelegramChannels) == 0 {
		return fmt.Errorf("no KOL targets configured (set KOL_TWITTER_HANDLES or KOL_TELEGRAM_CHANNELS)")
	}
	return nil
}

// --- Known service addresses for wash detection ---

var KnownFixedFloatAddresses = map[Chain][]string{
	ChainSolana:   {},
	ChainEthereum: {},
	ChainBase:     {},
	ChainBSC:      {},
}

var KnownBridgeContracts = map[Chain][]string{
	ChainSolana:   {"worm2ZoG2kUd4vFXhvjh93UUH596ayRfgQ2MgjNMTth"},
	ChainEthereum: {"0x3ee18B2214AFF97000D974cf647E7C347E8fa585", "0x4D73AdB72bC3DD368966edD0f0b2148401A178E2"},
	ChainBase:     {"0x4200000000000000000000000000000000000010"}, // Base Bridge
	ChainBSC:      {},
}

var ServiceLabels = map[string]string{
	"fixedfloat":     "fixedfloat",
	"fixed float":    "fixedfloat",
	"changenow":      "swap_service",
	"simpleswap":     "swap_service",
	"exch.cx":        "swap_service",
	"wormhole":       "bridge",
	"allbridge":      "bridge",
	"debridge":       "bridge",
	"portal bridge":  "bridge",
	"mayan":          "bridge",
	"across":         "bridge",
	"stargate":       "bridge",
	"hop protocol":   "bridge",
	"synapse":        "bridge",
	"layerzero":      "bridge",
	"tornado cash":   "mixer",
	"railgun":        "mixer",
}

// helpers
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func splitTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
