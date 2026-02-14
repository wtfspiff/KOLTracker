# 🔍 KOL Wallet Tracker

A Go-based intelligence tool that discovers and tracks crypto KOL (Key Opinion Leader) wallets by analyzing their social media posts and correlating with on-chain activity across **Solana**, **Ethereum**, **Base**, and **BSC**.

## What It Does

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────────┐
│  Social Media    │────>│   Extractor      │────>│  Token Mentions     │
│  Twitter / TG    │     │  Wallets, CAs,   │     │  Wallet Addresses   │
│  Monitoring      │     │  Tickers, Links  │     │  Bot Preferences    │
└─────────────────┘     └──────────────────┘     └────────┬────────────┘
                                                           │
        ┌──────────────────────────────────────────────────┘
        ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────────┐
│  Chain Scanner   │────>│  Pattern         │────>│  Wash Wallet        │
│  Solana/ETH/     │     │  Analyzer        │     │  Detection          │
│  Base/BSC        │     │  Trading profile │     │  Scoring & Alerts   │
└─────────────────┘     └──────────────────┘     └─────────────────────┘
        │                                                  │
        ▼                                                  ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────────┐
│  Fresh Wallet    │────>│  Amount Matcher  │────>│  Dashboard          │
│  Monitor         │     │  FixedFloat /    │     │  HTTP API +         │
│  Real-time catch │     │  Bridge tracing  │     │  Web UI             │
└─────────────────┘     └──────────────────┘     └─────────────────────┘
```

### Core Capabilities

1. **Social Media Monitoring** — Polls Twitter (API v2 + Nitter fallback) and Telegram (public channel scraping) for token mentions, wallet addresses, DEX links, and $TICKER references

2. **Multi-Chain Wallet Scanning** — Fetches swap history, transfers, and token holdings across Solana (Helius/Solscan), Ethereum (Etherscan), Base (Basescan), and BSC (BSCScan)

3. **KOL Trading Profile Building** — Automatically builds a fingerprint from:
   - Buy/sell size ranges and common amounts
   - Preferred DEX/aggregator (Jupiter, Raydium, Uniswap, PancakeSwap)
   - Priority fee / gas settings (bot fingerprint)
   - Timing patterns (do they buy before or after posting?)

4. **Wash Wallet Detection** — Scores suspected wallets on multiple signals:
   - Funded by FixedFloat, bridges, or mixers
   - Bought the same tokens the KOL mentioned
   - Buy timing correlates with KOL post times
   - Buy amounts match KOL's typical pattern
   - Uses the same DEX/bot as the KOL
   - Similar priority fee settings (bot fingerprint)

5. **Fresh Wallet Monitoring** — When a KOL posts about a token:
   - Watches for new buyers of that token in real-time
   - Checks if buyers are fresh wallets (< 7 days old)
   - Checks funding source (FixedFloat/bridge = highly suspicious)
   - Catches pre-buys (wallet bought BEFORE the KOL posted)

6. **Amount Matching** — Matches KOL outgoing transfers with wash wallet incoming amounts, accounting for FixedFloat's ~1-2% fee

## Architecture

```
cmd/
  tracker/
    main.go              # Entry point, orchestrates all components

pkg/
  config/
    config.go            # Configuration loading, chain definitions, known service addresses
  db/
    models.go            # All data models (KOL, Wallet, Transaction, WashCandidate, etc.)
    store.go             # SQLite database with full CRUD, schema, indexes
  extractor/
    extractor.go         # Regex-based extraction of addresses, CAs, tickers, links from text
  twitter/
    monitor.go           # Twitter API v2 + Nitter RSS polling, tweet processing
  telegram/
    monitor.go           # Telegram public channel scraping, message processing
  scanner/
    scanner.go           # Multi-chain tx scanning (Helius, Solscan, Etherscan, Basescan, BSCScan)
    helpers.go           # Wei conversion, address abbreviation, label matching
  analyzer/
    analyzer.go          # KOL profile building, wash wallet scoring, amount matching
  monitor/
    fresh_wallet.go      # Real-time fresh buyer detection after KOL token mentions
  dashboard/
    dashboard.go         # HTTP API + web UI for monitoring
```

## Setup

### 1. Prerequisites

- Go 1.22+
- API keys (at least some of these):

| Service | Purpose | Required? |
|---------|---------|-----------|
| Twitter Bearer Token | Tweet fetching via API | Optional (Nitter fallback) |
| Helius API Key | Parsed Solana transactions | Recommended for Solana |
| Solscan Pro API Key | Solana DeFi activities | Alternative to Helius |
| Birdeye API Key | Token price + recent buyers | Recommended |
| Etherscan API Key | ETH transaction scanning | Required for ETH |
| Basescan API Key | Base transaction scanning | Required for Base |
| BSCScan API Key | BSC transaction scanning | Required for BSC |

### 2. Configure

```bash
cp .env.example .env
# Edit .env with your API keys and KOL targets
```

Key configuration:
```env
# KOL targets
KOL_TWITTER_HANDLES=ansem,blknoiz06,CryptoGodJohn
KOL_TELEGRAM_CHANNELS=mychannel1,mychannel2

# Known wallets (address:chain:label)
KOL_KNOWN_WALLETS=5rEz...abc:solana:main,0x123...def:ethereum:trading

# Solana (pick one or both)
HELIUS_API_KEY=your_key
SOLSCAN_API_KEY=your_key

# EVM
ETHERSCAN_API_KEY=your_key
BASESCAN_API_KEY=your_key
BSCSCAN_API_KEY=your_key
```

### 3. Build & Run

```bash
cd kol-tracker
go mod tidy
go build -o kol-tracker ./cmd/tracker
./kol-tracker
```

### 4. Dashboard

Open `http://localhost:8080` for the web dashboard showing:
- Tracked wallets and their confidence scores
- Wash wallet candidates ranked by suspicion score
- Real-time alerts
- Funding flow matches

API endpoints:
```
GET /api/stats              # Database statistics
GET /api/kols               # KOL profiles with wallets
GET /api/wallets            # All tracked wallets
GET /api/wash-candidates    # Wash wallet candidates
GET /api/alerts             # Recent alerts
GET /api/funding-matches    # FixedFloat/bridge amount matches
```

## How Wash Wallet Detection Works

### Scoring Breakdown (0.0 - 1.0)

| Signal | Weight | Description |
|--------|--------|-------------|
| Token overlap | +0.1 per token (max 0.3) | Bought same tokens KOL mentioned |
| Timing correlation | +0.2 | >50% of buys within 2hrs of KOL posts |
| Pre-buy detection | +0.25 | Bought BEFORE KOL posted |
| Bot-speed buy | +0.2 | Bought within 30s of post |
| Amount pattern match | +0.15 | Buy sizes match KOL's typical amounts |
| Same DEX/bot | +0.1 | Uses same DEX as KOL |
| Fee fingerprint | +0.1 | Similar priority fee settings |
| FixedFloat funding | +0.3 | Funded by FixedFloat |
| Bridge funding | +0.2 | Funded via cross-chain bridge |
| Mixer funding | +0.35 | Funded via mixer (Tornado, Railgun) |
| New wallet (< 24h) | +0.2 | Wallet is less than a day old |
| New wallet (< 7d) | +0.1 | Wallet is less than a week old |

### Detection Flow

```
KOL tweets about $TOKEN
        │
        ▼
[Fresh Wallet Monitor activates for $TOKEN]
        │
        ▼
[Fetch all recent buyers of $TOKEN via Birdeye/Helius]
        │
        ▼
For each buyer:
  ├── Check wallet age (< 7 days = suspicious)
  ├── Check funding source (FixedFloat/bridge/mixer?)
  ├── Check buy timing (before/after KOL post?)
  ├── Match buy amount against KOL's typical sizes
  ├── Check if same DEX/aggregator as KOL
  └── Check priority fee fingerprint
        │
        ▼
[Score > 0.4 = ALERT]
[Score > 0.7 = CRITICAL ALERT]
```

## Extending

### Adding a New Chain
1. Add chain constant in `pkg/config/config.go`
2. Add RPC URL and explorer key in config
3. Add scanning logic in `pkg/scanner/scanner.go` (follow EVM pattern)
4. Add known service addresses for the chain

### Adding a New Service Label (e.g., new swap service)
Add to `ServiceLabels` map in `pkg/config/config.go`:
```go
var ServiceLabels = map[string]string{
    "newservice": "swap_service",
}
```

### Adding Known FixedFloat/Bridge Addresses
Update `KnownFixedFloatAddresses` or `KnownBridgeContracts` in config.

## Notes

- **CEX withdrawals** cannot be reliably identified as belonging to a specific KOL (they come from exchange hot wallets shared by millions of users). The tool focuses on identifiable funding paths: FixedFloat, bridges, mixers, and direct transfers.
- **Rate limits**: The tool includes built-in delays between API calls. Adjust intervals in `.env` if you hit rate limits.
- **Nitter fallback**: If Twitter API isn't configured, the tool falls back to Nitter RSS feeds for public tweets. Some Nitter instances may be unreliable.
- **Telegram**: Only public channels are supported via web scraping. For private groups, you'd need to add MTProto (gotd/td) authentication.
