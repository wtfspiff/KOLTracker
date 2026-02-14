package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
)

// DeepFundingTracer performs multi-hop tracing of funds through privacy services.
// It follows: KOL wallet → FixedFloat/Bridge/Mixer → Fresh wallet
// Also traces cross-chain flows where funds leave on one chain and arrive on another.
type DeepFundingTracer struct {
	scanner *Scanner
	store   *db.Store
	cfg     *config.Config
}

func NewDeepFundingTracer(sc *Scanner, store *db.Store, cfg *config.Config) *DeepFundingTracer {
	return &DeepFundingTracer{scanner: sc, store: store, cfg: cfg}
}

// TraceWalletFunding performs deep analysis of how a wallet was funded.
// Goes multiple hops back to find the original source.
func (t *DeepFundingTracer) TraceWalletFunding(ctx context.Context, address string, chain config.Chain, maxDepth int) (*FundingTrace, error) {
	trace := &FundingTrace{
		Address: address,
		Chain:   chain,
		Hops:    []FundingHop{},
	}

	visited := map[string]bool{address: true}
	return t.traceRecursive(ctx, trace, address, chain, 0, maxDepth, visited)
}

type FundingTrace struct {
	Address          string       `json:"address"`
	Chain            config.Chain `json:"chain"`
	Hops             []FundingHop `json:"hops"`
	OriginType       string       `json:"origin_type"`      // "fixedfloat","bridge","mixer","cex","wallet","unknown"
	OriginAddress    string       `json:"origin_address"`
	OriginChain      config.Chain `json:"origin_chain"`
	TotalAmount      float64      `json:"total_amount"`
	CrossChainFlow   bool         `json:"cross_chain_flow"`
	SuspicionLevel   string       `json:"suspicion_level"`   // "clean","low","medium","high","critical"
}

type FundingHop struct {
	FromAddress string       `json:"from_address"`
	ToAddress   string       `json:"to_address"`
	Amount      float64      `json:"amount"`
	Token       string       `json:"token"`
	TxHash      string       `json:"tx_hash"`
	Chain       config.Chain `json:"chain"`
	HopType     string       `json:"hop_type"`  // "direct","fixedfloat","bridge","mixer","cex_withdraw","unknown"
	ServiceName string       `json:"service_name"`
	Timestamp   time.Time    `json:"timestamp"`
	Depth       int          `json:"depth"`
}

func (t *DeepFundingTracer) traceRecursive(ctx context.Context, trace *FundingTrace, address string, chain config.Chain, depth, maxDepth int, visited map[string]bool) (*FundingTrace, error) {
	if depth >= maxDepth || ctx.Err() != nil {
		return trace, nil
	}

	funding, err := t.scanner.CheckFunding(ctx, address, chain)
	if err != nil {
		return trace, err
	}

	if funding.IsNewWallet && depth == 0 {
		trace.SuspicionLevel = "medium" // new wallet is somewhat suspicious
	}

	for _, src := range funding.FundingSources {
		hop := FundingHop{
			FromAddress: src.SourceAddress,
			ToAddress:   address,
			Amount:      src.Amount,
			Token:       src.Token,
			TxHash:      src.TxHash,
			Chain:       chain,
			HopType:     src.SourceType,
			Timestamp:   time.Unix(src.Timestamp, 0),
			Depth:       depth,
		}

		// Classify the service
		switch src.SourceType {
		case "fixedfloat":
			hop.ServiceName = "FixedFloat"
			trace.SuspicionLevel = maxSuspicion(trace.SuspicionLevel, "high")
		case "bridge":
			hop.ServiceName = t.identifyBridge(src.SourceAddress, chain)
			trace.SuspicionLevel = maxSuspicion(trace.SuspicionLevel, "medium")
			trace.CrossChainFlow = true
		case "mixer":
			hop.ServiceName = "Mixer"
			trace.SuspicionLevel = maxSuspicion(trace.SuspicionLevel, "critical")
		case "swap_service":
			hop.ServiceName = "SwapService"
			trace.SuspicionLevel = maxSuspicion(trace.SuspicionLevel, "high")
		}

		trace.Hops = append(trace.Hops, hop)
		trace.TotalAmount += src.Amount

		// If source is identifiable service, that's our origin
		if src.SourceType != "unknown" && src.SourceType != "" {
			trace.OriginType = src.SourceType
			trace.OriginAddress = src.SourceAddress
			trace.OriginChain = chain
		}

		// Recursively trace the source (only for unknown/direct transfers)
		if src.SourceType == "unknown" && !visited[src.SourceAddress] {
			visited[src.SourceAddress] = true
			t.traceRecursive(ctx, trace, src.SourceAddress, chain, depth+1, maxDepth, visited)
		}
	}

	// Also check cross-chain: look for bridge transfers arriving on OTHER chains
	if depth == 0 {
		for _, otherChain := range config.AllChains() {
			if otherChain == chain {
				continue
			}
			// Check if there's a wallet with matching address pattern on other chain
			t.checkCrossChainFunding(ctx, trace, address, chain, otherChain)
		}
	}

	if trace.SuspicionLevel == "" {
		trace.SuspicionLevel = "clean"
	}

	return trace, nil
}

func (t *DeepFundingTracer) checkCrossChainFunding(ctx context.Context, trace *FundingTrace, address string, sourceChain, destChain config.Chain) {
	// For EVM chains, the same address might exist on multiple chains
	if strings.HasPrefix(address, "0x") {
		otherFunding, err := t.scanner.CheckFunding(ctx, address, destChain)
		if err != nil || otherFunding.IsNewWallet {
			return
		}
		for _, src := range otherFunding.FundingSources {
			if src.SourceType == "bridge" {
				trace.CrossChainFlow = true
				trace.Hops = append(trace.Hops, FundingHop{
					FromAddress: src.SourceAddress,
					ToAddress:   address,
					Amount:      src.Amount,
					Token:       src.Token,
					TxHash:      src.TxHash,
					Chain:       destChain,
					HopType:     "bridge",
					ServiceName: t.identifyBridge(src.SourceAddress, destChain),
					Timestamp:   time.Unix(src.Timestamp, 0),
					Depth:       0,
				})
				trace.SuspicionLevel = maxSuspicion(trace.SuspicionLevel, "medium")
			}
		}
	}
}

// ScanForFixedFloatPatterns looks for the distinctive pattern of FixedFloat usage:
// 1. Round-ish amount sent from a wallet
// 2. Slightly smaller amount (minus ~1-2% fee) arrives at a fresh wallet
// 3. Time gap of 5-30 minutes typically
func (t *DeepFundingTracer) ScanForFixedFloatPatterns(ctx context.Context, kolID int64) ([]FixedFloatMatch, error) {
	var matches []FixedFloatMatch

	wallets, _ := t.store.GetWalletsForKOL(kolID)
	candidates, _ := t.store.GetWashCandidates(0.0)

	for _, w := range wallets {
		txs, _ := t.store.GetTransactionsForWallet(w.ID, 300)

		for _, tx := range txs {
			if tx.TxType != "transfer_out" || tx.AmountToken <= 0 {
				continue
			}

			// For each outgoing transfer, look for a matching incoming on a candidate
			for _, c := range candidates {
				if c.FundingAmount <= 0 {
					continue
				}

				// FixedFloat fee is typically 0.5-2.5%
				feePctLow := 0.3
				feePctHigh := 3.0

				expectedLow := tx.AmountToken * (1 - feePctHigh/100)
				expectedHigh := tx.AmountToken * (1 - feePctLow/100)

				if c.FundingAmount >= expectedLow && c.FundingAmount <= expectedHigh {
					// Amount matches FixedFloat fee range
					timeDiff := int64(0)
					if !tx.Timestamp.IsZero() && !c.FirstSeen.IsZero() {
						timeDiff = int64(c.FirstSeen.Sub(tx.Timestamp).Seconds())
					}

					// FixedFloat typically takes 5-45 minutes
					if timeDiff >= 0 && timeDiff <= 2700 { // 0-45 min
						feeAmt := tx.AmountToken - c.FundingAmount
						feePct := feeAmt / tx.AmountToken * 100

						match := FixedFloatMatch{
							KOLWallet:      w.Address,
							KOLChain:       w.Chain,
							OutgoingTx:     tx.TxHash,
							OutgoingAmount: tx.AmountToken,
							OutgoingToken:  tx.TokenSymbol,
							OutgoingTime:   tx.Timestamp,

							FreshWallet:    c.Address,
							FreshChain:     c.Chain,
							IncomingAmount: c.FundingAmount,
							IncomingToken:  c.FundingToken,
							IncomingTx:     c.FundingTx,

							FeeAmount:      feeAmt,
							FeePct:         feePct,
							TimeDiffSec:    timeDiff,
							Confidence:     calculateFFConfidence(feePct, timeDiff),

							IsCrossChain:   w.Chain != c.Chain,
						}

						matches = append(matches, match)

						log.Warn().
							Str("kol_wallet", abbrev(w.Address)).
							Float64("sent", tx.AmountToken).
							Str("fresh_wallet", abbrev(c.Address)).
							Float64("received", c.FundingAmount).
							Float64("fee_pct", feePct).
							Int64("time_gap_sec", timeDiff).
							Msg("🔴 FixedFloat pattern detected!")

						// Store as funding match
						t.store.InsertFundingMatch(db.FundingFlowMatch{
							SourceTx:        tx.TxHash,
							SourceChain:     w.Chain,
							SourceAmount:    tx.AmountToken,
							SourceToken:     tx.TokenSymbol,
							DestAddress:     c.Address,
							DestChain:       c.Chain,
							DestAmount:      c.FundingAmount,
							DestToken:       c.FundingToken,
							Service:         "fixedfloat",
							AmountDiffPct:   feePct,
							TimeDiffSeconds: timeDiff,
							MatchConfidence: match.Confidence,
						})
					}
				}
			}
		}
	}

	return matches, nil
}

type FixedFloatMatch struct {
	KOLWallet      string       `json:"kol_wallet"`
	KOLChain       config.Chain `json:"kol_chain"`
	OutgoingTx     string       `json:"outgoing_tx"`
	OutgoingAmount float64      `json:"outgoing_amount"`
	OutgoingToken  string       `json:"outgoing_token"`
	OutgoingTime   time.Time    `json:"outgoing_time"`

	FreshWallet    string       `json:"fresh_wallet"`
	FreshChain     config.Chain `json:"fresh_chain"`
	IncomingAmount float64      `json:"incoming_amount"`
	IncomingToken  string       `json:"incoming_token"`
	IncomingTx     string       `json:"incoming_tx"`

	FeeAmount      float64 `json:"fee_amount"`
	FeePct         float64 `json:"fee_pct"`
	TimeDiffSec    int64   `json:"time_diff_sec"`
	Confidence     float64 `json:"confidence"`
	IsCrossChain   bool    `json:"is_cross_chain"`
}

func calculateFFConfidence(feePct float64, timeDiff int64) float64 {
	// FixedFloat typical fee: 0.5-1.5%, typical time: 5-20 min
	feeConf := 1.0
	if feePct < 0.5 || feePct > 2.0 {
		feeConf = 0.6
	}

	timeConf := 1.0
	if timeDiff < 300 { // < 5 min (unusually fast)
		timeConf = 0.7
	} else if timeDiff > 1800 { // > 30 min (slow)
		timeConf = 0.8
	}

	return math.Min(feeConf*timeConf, 1.0)
}

func (t *DeepFundingTracer) identifyBridge(address string, chain config.Chain) string {
	addrLower := strings.ToLower(address)
	knownBridges := map[string]string{
		"worm2zog2kud4vfxhvjh93uuh596ayrfgq2mgjnmtth": "Wormhole",
		"0x3ee18b2214aff97000d974cf647e7c347e8fa585":   "Wormhole",
		"0x4200000000000000000000000000000000000010":     "Base Bridge",
		"0x49048044d57e1c92a77f79988d21fa8faf74e97e":    "Base Bridge",
	}
	if name, ok := knownBridges[addrLower]; ok {
		return name
	}
	return fmt.Sprintf("Bridge (%s)", string(chain))
}

func maxSuspicion(current, new string) string {
	levels := map[string]int{"": 0, "clean": 1, "low": 2, "medium": 3, "high": 4, "critical": 5}
	if levels[new] > levels[current] {
		return new
	}
	return current
}
