package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/ai"
	"github.com/kol-tracker/pkg/analyzer"
	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/dashboard"
	"github.com/kol-tracker/pkg/db"
	"github.com/kol-tracker/pkg/monitor"
	"github.com/kol-tracker/pkg/scanner"
	"github.com/kol-tracker/pkg/telegram"
	"github.com/kol-tracker/pkg/twitter"
)

func main() {
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}).With().Timestamp().Logger()
	log.Info().Msg("🔍 KOL Wallet Tracker starting...")

	cfg, err := config.Load()
	if err != nil { log.Fatal().Err(err).Msg("config load failed") }

	store, err := db.NewStore(cfg.DBPath)
	if err != nil { log.Fatal().Err(err).Msg("database init failed") }
	defer store.Close()

	// Seed from config
	for _, h := range cfg.KOLTwitterHandles { store.UpsertKOL(h, h, "") }
	for _, c := range cfg.KOLTelegramChannels { store.UpsertKOL(c, "", c) }
	for _, kw := range cfg.KOLKnownWallets {
		kols, _ := store.GetKOLs(); kolID := int64(1)
		if len(kols) > 0 { kolID = kols[0].ID }
		store.UpsertWallet(kolID, kw.Address, kw.Chain, kw.Label, 1.0, "config")
	}

	sc := scanner.New(cfg, store)
	an := analyzer.New(cfg, store)
	freshMon := monitor.NewFreshWalletMonitor(cfg, store, sc, an)
	twitterMon := twitter.NewMonitor(cfg, store)
	telegramMon := telegram.NewMonitor(cfg, store)

	cb := func(kolID int64, tokenAddr string, chain config.Chain, t time.Time) {
		freshMon.OnTokenMentioned(kolID, tokenAddr, chain, t)
	}
	twitterMon.SetTokenCallback(cb)
	telegramMon.SetTokenCallback(cb)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; log.Info().Msg("shutting down..."); cancel() }()

	errCh := make(chan error, 10)
	if len(cfg.KOLTwitterHandles) > 0 { go func() { errCh <- twitterMon.Run(ctx) }() }
	if len(cfg.KOLTelegramChannels) > 0 { go func() { errCh <- telegramMon.Run(ctx) }() }
	go func() { errCh <- freshMon.Run(ctx) }()
	go func() { errCh <- runScan(ctx, cfg, store, sc) }()
	go func() { errCh <- runAnalysis(ctx, cfg, store, an) }()

	// AI Engine (optional but recommended)
	aiEngine := ai.NewEngine(cfg, store)
	if aiEngine.IsEnabled() {
		go func() { errCh <- runAIAnalysis(ctx, cfg, aiEngine) }()
	}

	dash := dashboard.New(store, cfg, cfg.DashboardPort)
	go func() { errCh <- dash.Run() }()

	printSummary(cfg, store)
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && err != context.Canceled { log.Error().Err(err).Msg("error") }
	}
	log.Info().Msg("goodbye 👋")
}

func runScan(ctx context.Context, cfg *config.Config, store *db.Store, sc *scanner.Scanner) error {
	scanAll(ctx, store, sc)
	t := time.NewTicker(cfg.ChainScanInterval); defer t.Stop()
	for { select { case <-ctx.Done(): return ctx.Err(); case <-t.C: scanAll(ctx, store, sc) } }
}

func scanAll(ctx context.Context, store *db.Store, sc *scanner.Scanner) {
	ws, _ := store.GetAllTrackedAddresses()
	for _, w := range ws {
		if ctx.Err() != nil { return }
		if w.Confidence < 0.5 { continue }
		cnt, _ := sc.ScanWallet(ctx, w.ID, w.Address, w.Chain)
		if cnt > 0 { log.Info().Str("addr", w.Address[:8]+"...").Int("txs", cnt).Msg("📦 scanned") }
		linked, _ := sc.FindLinkedWallets(ctx, w.Address, w.Chain, 1)
		for _, l := range linked { store.UpsertWallet(w.KOLID, l.SourceAddress, l.Chain, "linked:"+l.SourceType, 0.4, "linked:"+w.Address[:8]) }
		time.Sleep(500 * time.Millisecond)
	}
}

func runAnalysis(ctx context.Context, cfg *config.Config, store *db.Store, an *analyzer.Analyzer) error {
	select { case <-ctx.Done(): return ctx.Err(); case <-time.After(30 * time.Second): }
	doAnalysis(store, an, cfg)
	t := time.NewTicker(cfg.PatternAnalysisInterval); defer t.Stop()
	for { select { case <-ctx.Done(): return ctx.Err(); case <-t.C: doAnalysis(store, an, cfg) } }
}

func doAnalysis(store *db.Store, an *analyzer.Analyzer, cfg *config.Config) {
	kols, _ := store.GetKOLs()
	for _, k := range kols {
		fp, _ := an.BuildKOLFingerprint(k.ID)
		if fp != nil && fp.TradeCount > 0 { log.Info().Str("kol", k.Name).Int("trades", fp.TradeCount).Msg("📊 profile") }
		cands, _ := store.GetWashCandidates(0.0)
		for _, c := range cands { if c.LinkedKOLID == k.ID || c.LinkedKOLID == 0 { an.ScoreWashCandidate(k.ID, c.Address, c.Chain) } }
		an.MatchFundingAmounts(k.ID, cfg.AmountMatchTolerancePct, 24)
	}
}

func printSummary(cfg *config.Config, store *db.Store) {
	stats, _ := store.GetStats()
	fmt.Println("\n" + strings.Repeat("═", 60))
	fmt.Println("  🔍 KOL WALLET TRACKER - RUNNING")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("  Twitter:   %v\n", cfg.KOLTwitterHandles)
	fmt.Printf("  Telegram:  %v\n", cfg.KOLTelegramChannels)
	fmt.Printf("  Chains:    Solana, Ethereum, Base, BSC\n")
	fmt.Printf("  Dashboard: http://localhost:%d\n", cfg.DashboardPort)
	aiStatus := "❌ Disabled (set ANTHROPIC_API_KEY or OPENAI_API_KEY)"
	if cfg.AnthropicAPIKey != "" { aiStatus = "✅ Anthropic Claude" }
	if cfg.OpenAIAPIKey != "" { aiStatus = "✅ OpenAI" }
	if cfg.OllamaURL != "" { aiStatus = "✅ Ollama (local)" }
	fmt.Printf("  AI Engine: %s\n", aiStatus)
	if stats != nil { fmt.Printf("  DB: %d KOLs, %d wallets, %d txs\n", stats["kol_profiles"], stats["tracked_wallets"], stats["wallet_transactions"]) }
	fmt.Println(strings.Repeat("═", 60) + "\n")
}

func runAIAnalysis(ctx context.Context, cfg *config.Config, engine *ai.Engine) error {
	// Wait for data to accumulate first
	select { case <-ctx.Done(): return ctx.Err(); case <-time.After(60 * time.Second): }
	log.Info().Msg("🤖 AI analysis engine starting...")

	engine.RunPeriodicAnalysis(ctx)

	t := time.NewTicker(cfg.AIAnalysisInterval); defer t.Stop()
	for {
		select {
		case <-ctx.Done(): return ctx.Err()
		case <-t.C:
			if err := engine.RunPeriodicAnalysis(ctx); err != nil {
				log.Error().Err(err).Msg("AI analysis error")
			}
		}
	}
}
