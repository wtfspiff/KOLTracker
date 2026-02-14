# KOL Tracker - Project Memory

## Overview
Go-based intelligence system that discovers and tracks crypto KOL wash wallets by correlating social media with on-chain transactions across Solana, Ethereum, Base, and BSC.

## Architecture
```
cmd/tracker/main.go          - Entry point, orchestrates all components
pkg/
  config/config.go           - Configuration, chain defs, known service addresses
  db/models.go               - All data models
  db/store.go                - SQLite WAL mode, full CRUD, indexes
  extractor/extractor.go     - Regex: addresses, CAs, tickers, DEX links, bot signals
  twitter/monitor.go         - Twitter API v2 + Nitter RSS fallback
  telegram/monitor.go        - Public channel scraping via t.me/s/
  scanner/scanner.go         - Multi-chain scanning (Helius, Etherscan, Basescan, BSCScan)
  scanner/helpers.go         - Wei conversion, utils
  scanner/deep_tracer.go     - Multi-hop FixedFloat/bridge/mixer detection
  analyzer/analyzer.go       - Advanced fingerprinting (13 scoring dimensions)
  monitor/fresh_wallet.go    - Real-time fresh buyer detection
  dashboard/server.go        - HTTP API with CORS, CRUD endpoints
  dashboard/frontend.go      - Embedded React SPA (dark terminal aesthetic)
```

## Wash Wallet Scoring (13 Dimensions)
1. Token overlap with KOL mentions (+0.3 max)
2. Timing correlation with KOL posts (+0.2)
3. Buy amount pattern match (+0.15)
4. DEX/router preference match (+0.1)
5. Gas/priority fee fingerprint (+0.15)
6. Funding source (FixedFloat +0.3, Bridge +0.2, Mixer +0.35)
7. Activity timing overlap (+0.1)
8. Deposit amount matching vs KOL outgoing (+0.15)
9. Same CEX usage (+0.1)
10. Bot/tool signature match (+0.1)
11. ENS/SNS naming link (+0.05)
12. Wallet age (< 24h +0.2, < 7d +0.1)
13. Chain preference match (+0.05)

## KOL Fingerprint
- Buy/sell size: percentiles, std dev, common amounts, buckets
- Gas: avg/median/common fees, Jito tip detection
- Timing: pre/post buy, hold time, day-of-week, 24h heatmap
- Token prefs: unique count, repeat buy %
- Deposit patterns, CEX profile, ENS/SNS, bot signatures

## Deep Funding Tracer
- Multi-hop recursive (configurable depth)
- FixedFloat: 0.3-3% fee, 0-45 min window
- Cross-chain flow detection
- Bridge ID (Wormhole, Base Bridge, etc.)
- Suspicion levels: clean/low/medium/high/critical

## Frontend
- React SPA in Go binary (no build step)
- Tabs: Overview, KOLs, Wallets, Wash Detection, Alerts
- Add KOL modal: Twitter/Telegram/known wallet
- KOL detail: wallets, wash candidates, alerts
- 8s polling, dark JetBrains Mono theme

## API
- GET /api/stats, /api/kols, /api/wallets, /api/wash-candidates, /api/alerts
- POST /api/kols/add (auto-starts monitoring)
- GET /api/kol/{id} (detail view)

## Config (.env)
- Helius, Solscan, Birdeye (Solana)
- Etherscan, Basescan, BSCScan (EVM)
- Twitter bearer / Nitter, Telegram API
- Thresholds: wash score min, amount tolerance, wallet age, timing windows

## Limitations
- CEX withdrawals hard to identify
- Nitter can be unreliable
- Telegram public channels only without MTProto
- No live price feeds yet (estimates)
