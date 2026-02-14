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
	// AI_PROVIDER: "anthropic" | "ollama" | "openai" (explicit selection)
	// If not set, auto-detects from available API keys
	AIProvider      string
	AnthropicAPIKey string
	OpenAIAPIKey    string
	OllamaURL       string // e.g. http://localhost:11434
	OllamaModel     string // model name for ollama (e.g. "llama3.1", "mistral", "deepseek-r1")
	OllamaAutoStart bool   // auto-pull model if not present

	// Model selection — supports per-task model routing
	// AI_MODEL: primary model for complex tasks (wallet study, discovery)
	// AI_MODEL_FAST: cheaper model for simple tasks (post analysis, reclassify)
	AIModel         string // primary model: claude-sonnet-4-20250514, claude-haiku-4-5-20251001, gpt-4o, llama3.1
	AIModelFast     string // fast/cheap model for simple tasks
	AIAnalysisInterval time.Duration
	AIMaxTokens     int    // max response tokens (default 4096)
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
		OllamaURL:       envOr("OLLAMA_URL", ""),
		OllamaModel:     envOr("OLLAMA_MODEL", "llama3.1"),
		OllamaAutoStart: envOr("OLLAMA_AUTO_PULL", "true") == "true",
		AIProvider:      os.Getenv("AI_PROVIDER"), // explicit: "anthropic","ollama","openai"
		AIModel:         envOr("AI_MODEL", ""),     // auto-resolved in AI engine
		AIModelFast:     envOr("AI_MODEL_FAST", ""), // auto-resolved in AI engine
		AIMaxTokens:     envInt("AI_MAX_TOKENS", 4096),
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
	// KOL targets can be empty — they can be added via dashboard at runtime.
	// But we should warn if no monitoring credentials are configured at all.
	hasTwitterAuth := c.TwitterUsername != "" || c.TwitterAuthToken != ""
	hasTelegramAuth := len(c.KOLTelegramChannels) > 0
	hasSolana := c.HeliusAPIKey != ""
	hasEVM := c.ExplorerKeys[ChainEthereum] != "" || c.ExplorerKeys[ChainBase] != "" || c.ExplorerKeys[ChainBSC] != ""

	if !hasTwitterAuth && !hasTelegramAuth && !hasSolana && !hasEVM {
		return fmt.Errorf("no API credentials configured — need at least one of: TWITTER credentials, HELIUS_API_KEY (Solana), or ETHERSCAN_API_KEY (EVM)")
	}
	return nil
}

// --- Known service addresses for wash detection ---

var KnownFixedFloatAddresses = map[Chain][]string{
	ChainSolana: {
		"FFixpaKkNRRKmRD1tFGqFrMBF26gKiNaaTPfbSdrFETS", // FixedFloat Solana hot wallet
		"FFSoLNFqJZuxyaqGG1GXMEfLEVf5pGAfRqVAWfTormYr", // FixedFloat Solana secondary
	},
	ChainEthereum: {
		"0x4E5B2e1dc63F6b91cb6Cd759936495434C7e972F", // FixedFloat ETH hot wallet
		"0xf1dA173228fcf015F43f3eA15aBBB51f0d8f1123", // FixedFloat ETH secondary
		"0x36928500Bc1dCd7af6a2B4008875CC336b927D57", // ChangeNow hot wallet
		"0x0D0707963952f2fBA59dD06f2b425ace40b492Fe", // SimpleSwap
	},
	ChainBase: {
		"0x4E5B2e1dc63F6b91cb6Cd759936495434C7e972F", // FixedFloat Base (same address)
	},
	ChainBSC: {
		"0x4E5B2e1dc63F6b91cb6Cd759936495434C7e972F", // FixedFloat BSC (same address)
		"0x0D0707963952f2fBA59dD06f2b425ace40b492Fe", // SimpleSwap BSC
	},
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

// KnownEVMAddresses maps well-known contract/hot wallet addresses to their service type.
// Used by identifyAddress() for EVM chains so we have Solscan-equivalent labeling.
var KnownEVMAddresses = map[string]string{
	// Uniswap
	"0x7a250d5630b4cf539739df2c5dacb4c659f2488d": "dex:uniswap_v2",
	"0xe592427a0aece92de3edee1f18e0157c05861564": "dex:uniswap_v3",
	"0x68b3465833fb72a70ecdf485e0e4c7bd8665fc45": "dex:uniswap_v3_router2",
	"0x3fc91a3afd70395cd496c647d5a6cc9d4b2b7fad": "dex:uniswap_universal",
	// PancakeSwap
	"0x10ed43c718714eb63d5aa57b78b54704e256024e": "dex:pancakeswap_v2",
	"0x13f4ea83d0bd40e75c8222255bc855a974568dd4": "dex:pancakeswap_v3",
	// SushiSwap
	"0xd9e1ce17f2641f24ae83637ab66a2cca9c378b9f": "dex:sushiswap",
	// 1inch
	"0x1111111254eeb25477b68fb85ed929f73a960582": "dex:1inch_v5",
	"0x111111125421ca6dc452d289314280a0f8842a65": "dex:1inch_v6",
	// 0x Protocol
	"0xdef1c0ded9bec7f1a1670819833240f027b25eff": "dex:0x_exchange",
	// Banana Gun
	"0x3328f7f4a1d1c57c35df56bbf0c9dcafca309c49": "bot:banana_gun",
	// Maestro
	"0x80a64c6d7f12c47b7c66c5b4e20e72bc0011fca7": "bot:maestro",
	// CEX Hot Wallets (Ethereum)
	"0x28c6c06298d514db089934071355e5743bf21d60": "cex:binance",
	"0x21a31ee1afc51d94c2efccaa2092ad1028285549": "cex:binance",
	"0xdfd5293d8e347dfe59e90efd55b2956a1343963d": "cex:binance",
	"0x56eddb7aa87536c09ccc2793473599fd21a8b17f": "cex:binance",
	"0x503828976d22510aad0201ac7ec88293211d23da": "cex:coinbase",
	"0xa9d1e08c7793af67e9d92fe308d5697fb81d3e43": "cex:coinbase",
	"0x71660c4005ba85c37ccec55d0c4493e66fe775d3": "cex:coinbase",
	"0x2faf487a4414fe77e2327f0bf4ae2a264a776ad2": "cex:ftx",
	"0xf89d7b9c864f589bbf53a82105107622b35eaa40": "cex:bybit",
	"0x1ab4973a48dc892cd9971ece8e01dcc7688f8f23": "cex:bybit",
	"0x0d0707963952f2fba59dd06f2b425ace40b492fe": "swap_service:simpleswap",
	"0x36928500bc1dcd7af6a2b4008875cc336b927d57": "swap_service:changenow",
}

// IdentifyKnownEVMAddress checks if an address is a known service on EVM chains.
func IdentifyKnownEVMAddress(address string) string {
	if label, ok := KnownEVMAddresses[strings.ToLower(address)]; ok {
		return label
	}
	return ""
}

// ClassifyEVMDEX returns the DEX name from an Etherscan "to" address in a swap tx.
func ClassifyEVMDEX(toAddr string) string {
	if label := IdentifyKnownEVMAddress(toAddr); strings.HasPrefix(label, "dex:") {
		return strings.TrimPrefix(label, "dex:")
	}
	return ""
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
