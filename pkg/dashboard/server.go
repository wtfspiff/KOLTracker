package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
)

type Dashboard struct {
	store *db.Store
	cfg   *config.Config
	port  int
}

func New(store *db.Store, cfg *config.Config, port int) *Dashboard {
	return &Dashboard{store: store, cfg: cfg, port: port}
}

func (d *Dashboard) Run() error {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/stats", cors(d.handleStats))
	mux.HandleFunc("/api/kols", cors(d.handleKOLs))
	mux.HandleFunc("/api/kols/add", cors(d.handleAddKOL))
	mux.HandleFunc("/api/wallets", cors(d.handleWallets))
	mux.HandleFunc("/api/wash-candidates", cors(d.handleWashCandidates))
	mux.HandleFunc("/api/alerts", cors(d.handleAlerts))
	mux.HandleFunc("/api/funding-matches", cors(d.handleFundingMatches))
	mux.HandleFunc("/api/kol/", cors(d.handleKOLDetail))

	// Serve frontend
	mux.HandleFunc("/", d.serveFrontend)

	addr := fmt.Sprintf(":%d", d.port)
	log.Info().Str("addr", addr).Msg("🌐 dashboard started")
	return http.ListenAndServe(addr, mux)
}

func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(200)
			return
		}
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
		WalletCount int               `json:"wallet_count"`
		Wallets     []db.TrackedWallet `json:"wallets"`
		AlertCount  int               `json:"alert_count"`
	}

	var result []kolView
	for _, k := range kols {
		wallets, _ := d.store.GetWalletsForKOL(k.ID)
		// count alerts
		alerts, _ := d.store.GetRecentAlerts(1000)
		ac := 0
		for _, a := range alerts {
			if a.KOLID == k.ID {
				ac++
			}
		}
		result = append(result, kolView{
			KOLProfile:  k,
			WalletCount: len(wallets),
			Wallets:     wallets,
			AlertCount:  ac,
		})
	}
	writeJSON(w, result)
}

func (d *Dashboard) handleAddKOL(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}

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
		http.Error(w, "invalid json", 400)
		return
	}

	if req.Name == "" {
		req.Name = req.TwitterHandle
		if req.Name == "" {
			req.Name = req.TelegramChannel
		}
	}

	kolID, err := d.store.UpsertKOL(req.Name, req.TwitterHandle, req.TelegramChannel)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Also add to runtime config so monitors pick it up
	if req.TwitterHandle != "" {
		d.cfg.KOLTwitterHandles = appendUniq(d.cfg.KOLTwitterHandles, req.TwitterHandle)
	}
	if req.TelegramChannel != "" {
		d.cfg.KOLTelegramChannels = appendUniq(d.cfg.KOLTelegramChannels, req.TelegramChannel)
	}

	// Add known wallets
	for _, kw := range req.KnownWallets {
		chain := config.Chain(kw.Chain)
		if chain == "" {
			if strings.HasPrefix(kw.Address, "0x") {
				chain = config.ChainEthereum
			} else {
				chain = config.ChainSolana
			}
		}
		d.store.UpsertWallet(kolID, kw.Address, chain, kw.Label, 1.0, "manual")
	}

	log.Info().Str("name", req.Name).Int64("id", kolID).Msg("➕ KOL added via dashboard")

	writeJSON(w, map[string]interface{}{
		"id":      kolID,
		"name":    req.Name,
		"status":  "ok",
		"message": "KOL added. Monitoring will start on next poll cycle.",
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
	if s := r.URL.Query().Get("min_score"); s != "" {
		minScore, _ = strconv.ParseFloat(s, 64)
	}
	candidates, _ := d.store.GetWashCandidates(minScore)
	writeJSON(w, candidates)
}

func (d *Dashboard) handleAlerts(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}
	alerts, _ := d.store.GetRecentAlerts(limit)
	writeJSON(w, alerts)
}

func (d *Dashboard) handleFundingMatches(w http.ResponseWriter, r *http.Request) {
	// TODO: add GetFundingMatches to store
	writeJSON(w, []interface{}{})
}

func (d *Dashboard) handleKOLDetail(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "not found", 404)
		return
	}
	kolID, _ := strconv.ParseInt(parts[3], 10, 64)

	wallets, _ := d.store.GetWalletsForKOL(kolID)
	patterns, _ := d.store.GetPatternsForKOL(kolID)
	alerts, _ := d.store.GetRecentAlerts(100)
	candidates, _ := d.store.GetWashCandidates(0.0)

	kolAlerts := []db.Alert{}
	for _, a := range alerts {
		if a.KOLID == kolID {
			kolAlerts = append(kolAlerts, a)
		}
	}

	kolCandidates := []db.WashWalletCandidate{}
	for _, c := range candidates {
		if c.LinkedKOLID == kolID {
			kolCandidates = append(kolCandidates, c)
		}
	}

	writeJSON(w, map[string]interface{}{
		"wallets":    wallets,
		"patterns":   patterns,
		"alerts":     kolAlerts,
		"candidates": kolCandidates,
	})
}

func appendUniq(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
