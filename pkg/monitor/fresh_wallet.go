package monitor

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/analyzer"
	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
	"github.com/kol-tracker/pkg/scanner"
)

// FreshWalletMonitor watches for newly funded wallets that buy tokens right after
// a KOL posts about them. This is the real-time catch mechanism.
type FreshWalletMonitor struct {
	cfg      *config.Config
	store    *db.Store
	scanner  *scanner.Scanner
	analyzer *analyzer.Analyzer

	mu       sync.RWMutex
	watches  map[string]*TokenWatch // "kolID:tokenAddr" -> watch
}

type TokenWatch struct {
	KOLID        int64
	TokenAddress string
	Chain        config.Chain
	MentionTime  time.Time
	Expires      time.Time
	Checked      map[string]bool // addresses already analyzed
}

func NewFreshWalletMonitor(cfg *config.Config, store *db.Store, sc *scanner.Scanner, an *analyzer.Analyzer) *FreshWalletMonitor {
	return &FreshWalletMonitor{
		cfg:      cfg,
		store:    store,
		scanner:  sc,
		analyzer: an,
		watches:  make(map[string]*TokenWatch),
	}
}

// OnTokenMentioned is called when a KOL mentions a token in social media.
// Starts watching for fresh wallet buyers.
func (m *FreshWalletMonitor) OnTokenMentioned(kolID int64, tokenAddr string, chain config.Chain, mentionTime time.Time) {
	key := fmt.Sprintf("%d:%s", kolID, tokenAddr)

	m.mu.Lock()
	m.watches[key] = &TokenWatch{
		KOLID:        kolID,
		TokenAddress: tokenAddr,
		Chain:        chain,
		MentionTime:  mentionTime,
		Expires:      mentionTime.Add(4 * time.Hour),
		Checked:      make(map[string]bool),
	}
	m.mu.Unlock()

	log.Info().
		Int64("kol", kolID).
		Str("token", abbrev(tokenAddr)).
		Str("chain", string(chain)).
		Msg("🔍 watching token for fresh buyers")
}

// Run continuously scans active watches for fresh wallet buyers.
func (m *FreshWalletMonitor) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.cfg.FreshBuyerScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			m.scanAllWatches(ctx)
		}
	}
}

func (m *FreshWalletMonitor) scanAllWatches(ctx context.Context) {
	m.mu.Lock()
	// Clean expired watches
	now := time.Now().UTC()
	for key, w := range m.watches {
		if now.After(w.Expires) {
			delete(m.watches, key)
			log.Debug().Str("key", key).Msg("watch expired")
		}
	}

	// Copy active watches
	active := make(map[string]*TokenWatch)
	for k, v := range m.watches {
		active[k] = v
	}
	m.mu.Unlock()

	for _, watch := range active {
		if ctx.Err() != nil {
			return
		}
		m.scanTokenBuyers(ctx, watch)
	}
}

func (m *FreshWalletMonitor) scanTokenBuyers(ctx context.Context, watch *TokenWatch) {
	buyers, err := m.scanner.GetRecentTokenBuyers(ctx, watch.TokenAddress, watch.Chain)
	if err != nil {
		log.Debug().Err(err).Str("token", abbrev(watch.TokenAddress)).Msg("failed to get buyers")
		return
	}

	for _, buyer := range buyers {
		if buyer.Address == "" || watch.Checked[buyer.Address] {
			continue
		}

		m.mu.Lock()
		watch.Checked[buyer.Address] = true
		m.mu.Unlock()

		// Analyze this buyer in background
		go m.analyzeBuyer(ctx, watch, buyer)
	}
}

func (m *FreshWalletMonitor) analyzeBuyer(ctx context.Context, watch *TokenWatch, buyer scanner.TokenBuyer) {
	score := 0.0
	signals := map[string]interface{}{}

	// 1. Check wallet age
	funding, err := m.scanner.CheckFunding(ctx, buyer.Address, watch.Chain)
	if err != nil {
		return
	}

	if funding.IsNewWallet {
		score += 0.15
		signals["brand_new_wallet"] = true
	} else if funding.FirstTxTime != nil {
		age := time.Since(*funding.FirstTxTime)
		if age < 24*time.Hour {
			score += 0.2
			signals["wallet_age_hours"] = age.Hours()
		} else if age < 7*24*time.Hour {
			score += 0.1
			signals["wallet_age_days"] = age.Hours() / 24
		}
	}

	// 2. Check funding source
	for _, src := range funding.FundingSources {
		switch src.SourceType {
		case "fixedfloat":
			score += 0.3
			signals["funded_by_fixedfloat"] = map[string]interface{}{
				"amount": src.Amount,
				"tx":     src.TxHash,
			}
			log.Warn().
				Str("buyer", abbrev(buyer.Address)).
				Str("chain", string(watch.Chain)).
				Msg("🚨 fresh buyer funded by FixedFloat!")
		case "bridge":
			score += 0.2
			signals["funded_by_bridge"] = true
		case "mixer":
			score += 0.35
			signals["funded_by_mixer"] = true
		}
	}

	// 3. Buy timing relative to KOL post
	timeDiff := buyer.Timestamp.Sub(watch.MentionTime).Seconds()
	if timeDiff < -60 {
		// Bought before KOL posted
		score += 0.25
		signals["pre_buy"] = map[string]interface{}{
			"seconds_before": math.Abs(timeDiff),
		}
	} else if timeDiff < 0 {
		score += 0.15
		signals["near_simultaneous"] = true
	} else if timeDiff < 30 {
		score += 0.2
		signals["bot_speed_buy"] = map[string]interface{}{
			"seconds_after": timeDiff,
		}
	} else if timeDiff < 300 {
		score += 0.05
		signals["fast_buy_seconds"] = timeDiff
	}

	// 4. Amount pattern match (needs KOL profile)
	// Will be done in full analysis pass

	score = math.Min(score, 1.0)

	if score < 0.2 {
		return // not suspicious enough
	}

	// Store as wash wallet candidate
	fundedBy := ""
	fundingType := "unknown"
	var fundingAmt float64
	var fundingTx string
	if len(funding.FundingSources) > 0 {
		src := funding.FundingSources[0]
		fundedBy = src.SourceAddress
		fundingType = src.SourceType
		fundingAmt = src.Amount
		fundingTx = src.TxHash
	}

	m.store.UpsertWashCandidate(db.WashWalletCandidate{
		Address:           buyer.Address,
		Chain:             watch.Chain,
		FundedBy:          fundedBy,
		FundingSourceType: fundingType,
		FundingAmount:     fundingAmt,
		FundingToken:      funding.NativeSymbol,
		FundingTx:         fundingTx,
		ConfidenceScore:   score,
		LinkedKOLID:       watch.KOLID,
		Notes:             fmt.Sprintf("token:%s", watch.TokenAddress),
	})

	// Also track this wallet for further analysis
	m.store.UpsertWallet(watch.KOLID, buyer.Address, watch.Chain, "wash_suspected", score,
		fmt.Sprintf("fresh_buyer:%s", watch.TokenAddress))

	severity := "info"
	if score >= 0.7 {
		severity = "critical"
	} else if score >= 0.5 {
		severity = "warning"
	}

	m.store.InsertAlert(watch.KOLID, "fresh_wash_wallet", severity,
		fmt.Sprintf("Fresh wallet %s bought KOL token (score: %.0f%%)", abbrev(buyer.Address), score*100),
		fmt.Sprintf("Signals: %v", signals),
		buyer.Address, watch.TokenAddress)

	log.Warn().
		Str("address", abbrev(buyer.Address)).
		Str("chain", string(watch.Chain)).
		Float64("score", score).
		Interface("signals", signals).
		Msg("⚠️ suspicious fresh buyer detected")
}

func abbrev(addr string) string {
	if len(addr) > 12 {
		return addr[:6] + "..." + addr[len(addr)-4:]
	}
	return addr
}
