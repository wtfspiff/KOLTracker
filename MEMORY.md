# KOL Wallet Tracker - Project Memory

## Overview
Go-based intelligence tool that discovers and tracks crypto KOL wallets by analyzing social media + on-chain activity across Solana, Ethereum, Base, BSC. Features an AI layer (Claude/OpenAI/Ollama) for intelligent post analysis and wallet discovery.

## Architecture
```
cmd/tracker/main.go          → Entry point, orchestrates all goroutines
pkg/config/config.go         → Config from .env (chains, APIs, thresholds, AI)
pkg/db/models.go             → All data models
pkg/db/store.go              → SQLite with WAL mode, full CRUD
pkg/extractor/extractor.go   → Regex extraction: wallets, CAs, tickers, DEX links, bot signals
pkg/twitter/monitor.go       → Twitter API v2 + Nitter RSS fallback
pkg/telegram/monitor.go      → Public channel scraping (t.me/s/)
pkg/scanner/scanner.go       → Multi-chain tx scanning (Helius, Etherscan, Basescan, BSCScan)
pkg/scanner/helpers.go       → Wei conversion, address utils
pkg/scanner/deep_tracer.go   → Deep funding tracing through FixedFloat/bridges/mixers
pkg/analyzer/analyzer.go     → KOL fingerprinting (13 dimensions) + wash wallet scoring
pkg/monitor/fresh_wallet.go  → Real-time fresh buyer detection after KOL token mentions
pkg/ai/engine.go             → LLM-powered analysis (post understanding, wallet discovery, reclassification)
pkg/dashboard/server.go      → HTTP API (CORS, REST endpoints, add KOL)
pkg/dashboard/frontend.go    → React SPA (dark terminal aesthetic)
```

## Supported Chains
- Solana (Helius API for parsed txs, Solscan for labels, Birdeye for buyers)
- Ethereum (Etherscan API)
- Base (Basescan API)
- BSC (BSCScan API)

## Key Features

### Social Monitoring
- Twitter: API v2 with rate limit handling + Nitter RSS fallback (no auth needed)
- Telegram: Public channel web scraping via t.me/s/
- Extracts: Solana addresses, EVM addresses, $TICKERS, DEX links (dexscreener, birdeye, pump.fun, photon, gmgn, bullx, axiom), trading bot mentions

### On-Chain Analysis
- Swap history (buy/sell detection via token transfer direction)
- Funding source identification (FixedFloat, bridges, mixers)
- Linked wallet discovery (transfer graph traversal)
- Cross-chain funding detection (same address on multiple EVM chains)

### Pattern Analyzer (13 scoring dimensions)
1. Token overlap with KOL mentions (0-0.30)
2. Buy timing correlation with posts (0-0.20)
3. Buy amount pattern matching (0-0.15)
4. DEX/router preference matching (0-0.10)
5. Gas/priority fee fingerprinting (0-0.15)
6. Funding source type (FixedFloat: 0.30, mixer: 0.35)
7. Activity timing patterns (hour-of-day)
8. Deposit amount matching (within 3% = FixedFloat fee)
9. CEX usage patterns
10. Bot/tool signature matching
11. ENS/SNS naming pattern links
12. Wallet age (< 24h: 0.20, < 7d: 0.10)
13. Chain preference matching

### Deep Funding Tracer
- Multi-hop recursive tracing (follows funds through intermediaries)
- FixedFloat pattern detection (0.5-2.5% fee, 5-45 min timing)
- Cross-chain bridge flow detection
- Mixer identification
- Suspicion level classification (clean → critical)

### AI Engine (LLM-Powered)
- **Post Analysis**: Intent detection (buy_call/shill/warning), sentiment, paid promo detection, pre-buy detection, wash trading risk assessment
- **Wallet Discovery**: Periodic analysis of all KOL data to suggest new wallets to track
- **Wallet Reclassification**: Re-evaluates wallet labels as more data accumulates
- **Wallet Relationship Analysis**: Compares two wallets across all behavioral dimensions
- Supports: Anthropic Claude (recommended), OpenAI GPT-4o, Ollama (local)

### Fresh Wallet Monitor
- Triggers when KOL mentions a token
- Watches for new buyers via Birdeye API
- Checks wallet age, funding source, buy timing, amount patterns
- Catches pre-buys (bought BEFORE KOL posted)

### Dashboard
- React SPA with dark crypto terminal aesthetic
- Real-time stats, alerts, wash candidates, wallet tracking
- Add KOL via UI (twitter handle or TG channel)
- API endpoints for all data (JSON)

## Database Schema (SQLite)
- kol_profiles: name, twitter_handle, telegram_channel
- tracked_wallets: address, chain, label, confidence, source
- social_posts: platform, content, extracted tokens/wallets/links
- token_mentions: token_address, symbol, chain, mentioned_at
- wallet_transactions: tx_hash, chain, type, token, amounts, platform, priority_fee
- wash_wallet_candidates: address, funding_source, signals, confidence_score
- trading_patterns: pattern_type, pattern_data (JSON)
- funding_flow_matches: source→dest amount matching with confidence
- alerts: type, severity, title, description

## Required API Keys
| Service | Purpose | Required? |
|---------|---------|-----------|
| Helius | Solana parsed txs | Recommended |
| Solscan Pro | Solana labels | Alternative |
| Birdeye | Token prices + buyers | Recommended |
| Etherscan | ETH scanning | For ETH |
| Basescan | Base scanning | For Base |
| BSCScan | BSC scanning | For BSC |
| Twitter Bearer | Tweet fetching | Optional (Nitter fallback) |
| Anthropic/OpenAI | AI analysis | Optional but recommended |

## Known Limitations
- CEX withdrawals can't be attributed to specific users
- Nitter instances can be unreliable
- Pattern matching needs 50+ trades for reliable KOL profile
- FixedFloat addresses change - need periodic updates
- AI analysis costs ~$0.01-0.05 per post analysis call
- Telegram only supports public channels without MTProto auth

## Development History
- v1: Python prototype with basic regex extraction
- v2: Rewritten in Go with multi-chain support (ETH/Base/BSC + Solana)
- v3: Added advanced 13-dimension pattern analyzer
- v4: Added deep funding tracer for FixedFloat/bridge/mixer detection
- v5: Added AI/LLM engine for intelligent post analysis and wallet discovery
- v6: Added React frontend dashboard with KOL management UI

## Recent Changes (v2)

### Twitter Private API (imperatrona/twitter-scraper)
- Replaced public Twitter API v2 with reverse-engineered frontend API
- Uses `github.com/imperatrona/twitter-scraper` (MIT licensed)
- Auth: username/password login, auth_token+ct0 cookies, or saved session cookies
- Cookie persistence in `twitter_cookies.json` for session survival across restarts
- No rate limits, no API key costs, real-time access to any public account's tweets

### Immediate Backfill on KOL Add
- When a KOL is added via the dashboard, the system IMMEDIATELY:
  1. Backfills last 200 tweets from Twitter (via private API)
  2. Backfills last 10 pages (~200 messages) from Telegram (via web scraping)
  3. Studies all provided known wallets (deep transaction analysis)
- All 3 jobs run as background goroutines so the API responds instantly
- User sees a toast notification confirming background work has started

### Wallet Study Engine (pkg/scanner/wallet_study.go)
- Deep analysis when a wallet is manually added to a KOL:
  1. Fetches full transaction history via chain scanner
  2. Traces all direct transfers to find connected wallets
  3. Traces funding sources (FixedFloat, bridges, mixers, CEX)
  4. Checks cross-chain presence (same EVM address on ETH/Base/BSC)
  5. Second-degree linked wallet discovery (friends of friends)
  6. Co-trader detection (wallets buying same tokens in similar timeframes)
- Triggered immediately when wallet is added via `/api/wallets/add`

### Add Wallet Endpoint (POST /api/wallets/add)
- New dedicated API endpoint for adding wallets to existing KOLs
- Request: `{kol_id, address, chain, label}`
- Auto-detects chain from address format (0x → EVM, else → Solana)
- Immediately triggers WalletStudyEngine in background

### Frontend Updates
- Complete React SPA rebuild with consistent JSX/Babel
- **Add Wallet Modal**: accessible from KOL detail view, explains AI will study the wallet
- **Add KOL Modal**: shows note about immediate backfill of tweets + TG
- **Toast notifications**: confirms background jobs have started
- Auto-chain detection when typing wallet address (0x → auto-selects ETH)
- Chain filter buttons on Wallets tab

### Dashboard Wiring
- `Dashboard.SetMonitors(twitterMon, telegramMon, studyEngine)` wires up references
- `twitterMon.AddHandle(handle)` / `telegramMon.AddChannel(channel)` for runtime additions
- Background goroutines with 5-10 minute timeouts for backfill/study jobs

### Config Additions
```
TWITTER_USERNAME=       # for login
TWITTER_PASSWORD=
TWITTER_EMAIL=          # if email verification enabled
TWITTER_AUTH_TOKEN=     # alternative: browser cookie
TWITTER_CSRF_TOKEN=     # alternative: browser cookie
TWITTER_COOKIE_FILE=    # session persistence
```
