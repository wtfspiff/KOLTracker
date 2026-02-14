package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
	"github.com/kol-tracker/pkg/scanner"
	"github.com/kol-tracker/pkg/telegram"
	"github.com/kol-tracker/pkg/twitter"
)

type Dashboard struct {
	store       *db.Store
	cfg         *config.Config
	port        int
	twitterMon  *twitter.Monitor
	telegramMon *telegram.Monitor
	studyEngine *scanner.WalletStudyEngine
	aiInfo      func() map[string]interface{} // returns AI provider info
}

func New(store *db.Store, cfg *config.Config, port int) *Dashboard {
	return &Dashboard{store: store, cfg: cfg, port: port}
}

// SetMonitors wires up the social monitors and wallet study engine so the
// dashboard can trigger immediate backfill + wallet study when a KOL is added.
func (d *Dashboard) SetMonitors(tw *twitter.Monitor, tg *telegram.Monitor, study *scanner.WalletStudyEngine) {
	d.twitterMon = tw
	d.telegramMon = tg
	d.studyEngine = study
}

func (d *Dashboard) SetAIInfo(fn func() map[string]interface{}) {
	d.aiInfo = fn
}

func (d *Dashboard) Run() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/stats", cors(d.handleStats))
	mux.HandleFunc("/api/kols", cors(d.handleKOLs))
	mux.HandleFunc("/api/kols/add", cors(d.handleAddKOL))
	mux.HandleFunc("/api/wallets", cors(d.handleWallets))
	mux.HandleFunc("/api/wallets/add", cors(d.handleAddWallet))
	mux.HandleFunc("/api/wash-candidates", cors(d.handleWashCandidates))
	mux.HandleFunc("/api/alerts", cors(d.handleAlerts))
	mux.HandleFunc("/api/funding-matches", cors(d.handleFundingMatches))
	mux.HandleFunc("/api/kol/", cors(d.handleKOLDetail))
	mux.HandleFunc("/api/ai/info", cors(d.handleAIInfo))

	mux.HandleFunc("/", d.serveFrontend)

	addr := fmt.Sprintf(":%d", d.port)
	log.Info().Str("addr", addr).Msg("🌐 dashboard started")
	return http.ListenAndServe(addr, mux)
}

func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (d *Dashboard) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, _ := d.store.GetStats()
	writeJSON(w, stats)
}

func (d *Dashboard) handleKOLs(w http.ResponseWriter, r *http.Request) {
	kols, _ := d.store.GetKOLs()
	type kolView struct {
		db.KOLProfile
		WalletCount int                `json:"wallet_count"`
		Wallets     []db.TrackedWallet `json:"wallets"`
		AlertCount  int                `json:"alert_count"`
	}
	var result []kolView
	for _, k := range kols {
		wallets, _ := d.store.GetWalletsForKOL(k.ID)
		alerts, _ := d.store.GetRecentAlerts(1000)
		ac := 0
		for _, a := range alerts {
			if a.KOLID == k.ID { ac++ }
		}
		result = append(result, kolView{KOLProfile: k, WalletCount: len(wallets), Wallets: wallets, AlertCount: ac})
	}
	writeJSON(w, result)
}

// handleAddKOL adds a new KOL, immediately starts backfilling tweets/TG,
// and studies any provided known wallets.
func (d *Dashboard) handleAddKOL(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, "POST only", 405); return }

	body, _ := io.ReadAll(r.Body)
	var req struct {
		Name            string `json:"name"`
		TwitterHandle   string `json:"twitter_handle"`
		TelegramChannel string `json:"telegram_channel"`
		KnownWallets    []struct {
			Address string `json:"address"`
			Chain   string `json:"chain"`
			Label   string `json:"label"`
		} `json:"known_wallets"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", 400); return
	}

	name := req.Name
	if name == "" { name = req.TwitterHandle }
	if name == "" { name = req.TelegramChannel }
	if name == "" { http.Error(w, "name or handle required", 400); return }

	kolID, err := d.store.UpsertKOL(name, req.TwitterHandle, req.TelegramChannel)
	if err != nil { http.Error(w, err.Error(), 500); return }

	// Add to runtime config
	if req.TwitterHandle != "" {
		d.cfg.KOLTwitterHandles = appendUniq(d.cfg.KOLTwitterHandles, req.TwitterHandle)
		if d.twitterMon != nil {
			d.twitterMon.AddHandle(req.TwitterHandle)
		}
	}
	if req.TelegramChannel != "" {
		d.cfg.KOLTelegramChannels = appendUniq(d.cfg.KOLTelegramChannels, req.TelegramChannel)
		if d.telegramMon != nil {
			d.telegramMon.AddChannel(req.TelegramChannel)
		}
	}

	// Add known wallets
	for _, kw := range req.KnownWallets {
		chain := config.Chain(kw.Chain)
		if chain == "" {
			if strings.HasPrefix(kw.Address, "0x") { chain = config.ChainEthereum } else { chain = config.ChainSolana }
		}
		label := kw.Label
		if label == "" { label = "manual" }
		d.store.UpsertWallet(kolID, kw.Address, chain, label, 1.0, "manual")
	}

	log.Info().Str("name", name).Int64("id", kolID).
		Int("wallets", len(req.KnownWallets)).Msg("➕ KOL added via dashboard")

	// ── IMMEDIATE BACKGROUND JOBS ──
	// Kick off backfill + wallet study in background goroutines
	// so the API responds instantly while work happens async.

	// 1. Backfill Twitter history
	if req.TwitterHandle != "" && d.twitterMon != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			log.Info().Str("handle", req.TwitterHandle).Msg("🔄 starting immediate twitter backfill")
			if err := d.twitterMon.BackfillKOL(ctx, kolID, req.TwitterHandle, 200); err != nil {
				log.Warn().Err(err).Str("handle", req.TwitterHandle).Msg("twitter backfill error")
			}
		}()
	}

	// 2. Backfill Telegram history
	if req.TelegramChannel != "" && d.telegramMon != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			log.Info().Str("channel", req.TelegramChannel).Msg("🔄 starting immediate telegram backfill")
			if err := d.telegramMon.BackfillChannel(ctx, kolID, req.TelegramChannel, 10); err != nil {
				log.Warn().Err(err).Str("channel", req.TelegramChannel).Msg("telegram backfill error")
			}
		}()
	}

	// 3. Study all provided known wallets
	if len(req.KnownWallets) > 0 && d.studyEngine != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			log.Info().Int64("kol", kolID).Int("wallets", len(req.KnownWallets)).
				Msg("🔬 starting immediate wallet study")
			d.studyEngine.StudyAllWalletsForKOL(ctx, kolID)
		}()
	}

	writeJSON(w, map[string]interface{}{
		"id":      kolID,
		"name":    name,
		"status":  "ok",
		"message": "KOL added. Backfilling history & studying wallets in background.",
		"jobs":    []string{"twitter_backfill", "telegram_backfill", "wallet_study"},
	})
}

// handleAddWallet adds a known wallet to an existing KOL and immediately studies it.
func (d *Dashboard) handleAddWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, "POST only", 405); return }

	body, _ := io.ReadAll(r.Body)
	var req struct {
		KOLID   int64  `json:"kol_id"`
		Address string `json:"address"`
		Chain   string `json:"chain"`
		Label   string `json:"label"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", 400); return
	}

	if req.Address == "" { http.Error(w, "address required", 400); return }
	if req.KOLID == 0 { http.Error(w, "kol_id required", 400); return }

	chain := config.Chain(req.Chain)
	if chain == "" {
		if strings.HasPrefix(req.Address, "0x") { chain = config.ChainEthereum } else { chain = config.ChainSolana }
	}
	label := req.Label
	if label == "" { label = "manual" }

	d.store.UpsertWallet(req.KOLID, req.Address, chain, label, 1.0, "manual")

	log.Info().Int64("kol", req.KOLID).Str("address", req.Address[:min(12,len(req.Address))]).
		Str("chain", string(chain)).Msg("👛 wallet added via dashboard")

	// Immediately study the wallet in background
	if d.studyEngine != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			log.Info().Str("wallet", req.Address[:min(12,len(req.Address))]).Msg("🔬 starting immediate wallet study")
			result, err := d.studyEngine.StudyWallet(ctx, req.KOLID, req.Address, chain)
			if err != nil {
				log.Warn().Err(err).Msg("wallet study error")
				return
			}
			log.Info().Int("txs", result.TransactionsFound).
				Int("linked", len(result.LinkedWallets)).
				Int("cross_chain", len(result.CrossChainAddrs)).
				Msg("🔬 wallet study complete")
		}()
	}

	writeJSON(w, map[string]interface{}{
		"status":  "ok",
		"address": req.Address,
		"chain":   string(chain),
		"message": "Wallet added. Studying transactions and finding linked wallets in background.",
	})
}

func (d *Dashboard) handleWallets(w http.ResponseWriter, r *http.Request) {
	kolIDStr := r.URL.Query().Get("kol_id")
	if kolIDStr != "" {
		kolID, _ := strconv.ParseInt(kolIDStr, 10, 64)
		wallets, _ := d.store.GetWalletsForKOL(kolID)
		writeJSON(w, wallets)
		return
	}
	wallets, _ := d.store.GetAllTrackedAddresses()
	writeJSON(w, wallets)
}

func (d *Dashboard) handleWashCandidates(w http.ResponseWriter, r *http.Request) {
	minScore := 0.0
	if s := r.URL.Query().Get("min_score"); s != "" { minScore, _ = strconv.ParseFloat(s, 64) }
	candidates, _ := d.store.GetWashCandidates(minScore)
	writeJSON(w, candidates)
}

func (d *Dashboard) handleAlerts(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" { limit, _ = strconv.Atoi(l) }
	alerts, _ := d.store.GetRecentAlerts(limit)
	writeJSON(w, alerts)
}

func (d *Dashboard) handleFundingMatches(w http.ResponseWriter, r *http.Request) {
	matches, err := d.store.GetFundingMatches(100)
	if err != nil || matches == nil {
		writeJSON(w, []interface{}{})
		return
	}
	writeJSON(w, matches)
}

func (d *Dashboard) handleAIInfo(w http.ResponseWriter, r *http.Request) {
	if d.aiInfo != nil {
		writeJSON(w, d.aiInfo())
	} else {
		writeJSON(w, map[string]interface{}{"enabled": false})
	}
}

func (d *Dashboard) handleKOLDetail(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 { http.Error(w, "not found", 404); return }
	kolID, _ := strconv.ParseInt(parts[3], 10, 64)

	wallets, _ := d.store.GetWalletsForKOL(kolID)
	patterns, _ := d.store.GetPatternsForKOL(kolID)
	alerts, _ := d.store.GetRecentAlerts(100)
	candidates, _ := d.store.GetWashCandidates(0.0)

	kolAlerts := []db.Alert{}
	for _, a := range alerts { if a.KOLID == kolID { kolAlerts = append(kolAlerts, a) } }
	kolCandidates := []db.WashWalletCandidate{}
	for _, c := range candidates { if c.LinkedKOLID == kolID { kolCandidates = append(kolCandidates, c) } }

	writeJSON(w, map[string]interface{}{
		"wallets": wallets, "patterns": patterns, "alerts": kolAlerts, "candidates": kolCandidates,
	})
}

func appendUniq(s []string, v string) []string {
	for _, x := range s { if x == v { return s } }
	return append(s, v)
}

func min(a, b int) int { if a < b { return a }; return b }
