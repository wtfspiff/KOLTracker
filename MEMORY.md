# KOL Tracker - Project Memory

## Overview
KOL Wallet Tracker is a Go-based intelligence system that discovers and tracks crypto KOL (Key Opinion Leader) wallets by correlating social media activity with on-chain transactions across Solana, Ethereum, Base, and BSC. It detects wash wallets — secret wallets KOLs use to front-run their own calls.

## Architecture

### Language & Stack
- **Language**: Go 1.22+
- **Database**: SQLite (WAL mode, embedded)
- **Frontend**: React SPA (served as embedded HTML from Go)
- **No external frameworks**: Pure stdlib HTTP server + zerolog for logging

### Directory Structure
```
kol-tracker/
├── cmd/tracker/main.go          # Entry point, orchestrates all components
├── go.mod                       # Go module definition
├── .env.example                 # Configuration template
├── README.md                    # User-facing documentation
├── MEMORY.md                    # This file - project memory
└── pkg/
    ├── config/config.go         # Config loading, chain definitions, known service addresses
    ├── db/
    │   ├── models.go            # All data models
    │   └── store.go             # SQLite schema + CRUD operations
    ├── extractor/extractor.go   # Regex extraction of addresses, tokens, links from text
    ├── twitter/monitor.go       # Twitter API v2 + Nitter RSS fallback
    ├── telegram/monitor.go      # Telegram public channel web scraping
    ├── scanner/
    │   ├── scanner.go           # Multi-chain tx scanning (Solana + EVM)
    │   ├── helpers.go           # Wei conversion, utilities
    │   └── deep_tracer.go       # Deep funding tracing through FixedFloat/bridges/mixers
    ├── analyzer/analyzer.go     # Advanced KOL fingerprinting + wash wallet scoring (13 factors)
    ├── monitor/fresh_wallet.go  # Real-time fresh buyer detection after KOL posts
    └── dashboard/
        ├── server.go            # HTTP API server with CRUD endpoints
        └── frontend.go          # Embedded React SPA (dark crypto terminal theme)
```

### Component Flow
```
1. Twitter/Telegram Monitors → poll social media for KOL posts
2. Extractor → parse text for wallet addresses, token CAs, $TICKERs, DEX links
3. Token mentions stored → triggers Fresh Wallet Monitor
4. Fresh Wallet Monitor → watches for new buyers of mentioned tokens
5. Scanner → fetches on-chain data (Helius for Solana, Etherscan/Basescan/BSCScan for EVM)
6. Deep Funding Tracer → traces funding through FixedFloat, bridges, mixers (multi-hop)
7. Analyzer → builds KOL trading fingerprint, scores wash wallet candidates on 13 dimensions
8. Dashboard → serves React frontend + JSON API at localhost:8080
```

### Database Schema (SQLite)
Tables:
- `kol_profiles` - KOL identities (name, twitter, telegram)
- `tracked_wallets` - All discovered wallets with confidence scores
- `social_posts` - Stored tweets/TG messages with extracted data
- `token_mentions` - Token CAs and tickers mentioned by KOLs
- `wallet_transactions` - On-chain swap/transfer history
- `wash_wallet_candidates` - Suspected wash wallets with scoring signals
- `trading_patterns` - KOL fingerprint data (JSON blobs)
- `funding_flow_matches` - FixedFloat/bridge amount matching results
- `alerts` - Generated alerts with severity levels

### API Endpoints
```
GET  /api/stats              # Database statistics
GET  /api/kols               # All KOL profiles with wallet counts
POST /api/kols/add           # Add new KOL (name, twitter, telegram, known_wallets)
GET  /api/wallets            # All tracked wallets (filterable by kol_id)
GET  /api/wash-candidates    # Wash wallet candidates (filterable by min_score)
GET  /api/alerts             # Recent alerts (filterable by limit)
GET  /api/kol/{id}           # KOL detail (wallets, patterns, candidates, alerts)
GET  /api/funding-matches    # FixedFloat/bridge funding matches
GET  /                       # React frontend
```

## Key Technical Decisions

### Wash Wallet Detection - 13 Scoring Factors
1. **Token overlap** (0-0.30) - Bought same tokens KOL mentioned
2. **Timing correlation** (0-0.20) - Buys correlate with KOL post times
3. **Buy amount pattern** (0-0.15) - Buy sizes match KOL's typical amounts
4. **DEX/Router match** (0-0.10) - Uses same DEX as KOL
5. **Gas/fee fingerprint** (0-0.15) - Similar priority fee settings (bot fingerprint)
6. **Funding source** (0-0.35) - FixedFloat/bridge/mixer funding
7. **Activity timing** (0-0.10) - Same hours-of-day activity pattern
8. **Deposit amount match** (0-0.15) - KOL outgoing ≈ wash wallet incoming (minus fees)
9. **CEX match** (0-0.10) - Same CEX withdrawal patterns
10. **Bot/tool signature** (0-0.10) - Same trading bot used
11. **ENS/naming pattern** (0-0.05) - Related naming patterns
12. **Wallet age** (0-0.20) - Fresh wallets are more suspicious
13. **Chain preference** (0-0.05) - Operates on KOL's preferred chain

### KOL Fingerprinting (Advanced)
The fingerprint captures:
- Buy/Sell size patterns with statistical analysis (mean, median, stddev, percentiles, buckets, common amounts)
- Gas/priority fee profile with Jito tip detection (Solana MEV)
- Timing behavior (pre-buy vs post-buy, hold times, day-of-week)
- Token preferences (unique count, repeat buy %, pump.fun vs raydium preference)
- CEX usage patterns and deposit amounts
- ENS/SNS naming patterns
- 24-hour activity heatmap
- Chain preference distribution

### Deep Funding Tracer
- Multi-hop recursive tracing (configurable depth)
- FixedFloat pattern detection (0.5-2.5% fee, 5-45 min time gap)
- Cross-chain flow detection (same EVM address on multiple chains)
- Bridge identification (Wormhole, Base Bridge, etc.)
- Suspicion level classification (clean → low → medium → high → critical)

### Social Media Monitoring
- Twitter: API v2 with bearer token, falls back to Nitter RSS
- Telegram: Public channel web scraping (t.me/s/channel), no auth needed
- Bot detection: Identifies references to BonkBot, Trojan, Maestro, Banana Gun, Photon, BullX, Axiom, Bloom, PepeBoost
- Buy/sell signal detection from text

### Frontend Design
- Dark crypto terminal aesthetic (JetBrains Mono + Space Grotesk fonts)
- Real-time auto-refresh (10s polling)
- Chain-colored badges (purple=Solana, blue=ETH, blue=Base, yellow=BSC)
- Score visualization with color coding (red=critical, orange=warning, gray=low)
- Signal tags showing which detection factors matched
- Add KOL modal with wallet input and chain selection
- Tab navigation: Overview, KOLs, Wallets, Wash Detection, Alerts

## Configuration Requirements

### Required APIs (at minimum)
- Helius API Key (Solana transaction parsing)
- One of: Etherscan/Basescan/BSCScan API keys (EVM chains)

### Recommended APIs
- Birdeye API Key (real-time token buyer lists)
- Twitter Bearer Token (reliable tweet fetching)

### Optional
- Solscan Pro API Key (alternative Solana data)
- Telegram API credentials (for private channels)

## Known Limitations
- CEX withdrawals can't be attributed to specific users (shared hot wallets)
- Nitter instances may be unreliable/offline
- Telegram monitoring limited to public channels without MTProto auth
- Pattern matching improves with data (need 50+ trades for reliable fingerprint)
- DexScreener doesn't provide individual buyer addresses (need Birdeye for that)
- EVM DEX swap detection is heuristic (based on stablecoin transfers)

## Future Improvements
- [ ] WebSocket real-time updates instead of polling
- [ ] Arkham/Nansen label integration for better address identification
- [ ] Helius webhook subscriptions for real-time Solana monitoring
- [ ] Internal transaction / trace analysis for EVM DEX parsing
- [ ] Discord/Telegram webhook alerts
- [ ] Machine learning model for pattern refinement
- [ ] Multi-KOL cross-reference (same wash wallet used by multiple KOLs)
- [ ] Historical backfill capability
- [ ] Export/report generation
