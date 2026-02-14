package scanner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
)

// WalletStudyEngine performs deep analysis of manually added wallets to discover
// all linked wallets, trading patterns, and associated addresses.
// When a user adds a known wallet for a KOL, this engine:
// 1. Fetches full transaction history
// 2. Traces all outgoing transfers to find connected wallets
// 3. Traces all incoming transfers to find funding sources
// 4. Checks cross-chain presence (same EVM address on ETH/Base/BSC)
// 5. Finds wallets that received from the same funding sources
// 6. Discovers wallets that traded the same tokens in similar timeframes
type WalletStudyEngine struct {
	scanner *Scanner
	store   *db.Store
	cfg     *config.Config
}

func NewWalletStudyEngine(sc *Scanner, store *db.Store, cfg *config.Config) *WalletStudyEngine {
	return &WalletStudyEngine{scanner: sc, store: store, cfg: cfg}
}

// StudyResult contains all discoveries from analyzing a wallet.
type StudyResult struct {
	WalletAddress   string              `json:"wallet_address"`
	Chain           config.Chain        `json:"chain"`
	TransactionsFound int              `json:"transactions_found"`
	LinkedWallets   []LinkedWallet      `json:"linked_wallets"`
	FundingSources  []db.FundingSource  `json:"funding_sources"`
	CrossChainAddrs []CrossChainAddr    `json:"cross_chain_addrs"`
	CoTraders       []CoTrader          `json:"co_traders"`
}

type LinkedWallet struct {
	Address    string       `json:"address"`
	Chain      config.Chain `json:"chain"`
	Relation   string       `json:"relation"` // "sent_to","received_from","shared_funding","co_trader"
	Amount     float64      `json:"amount"`
	TxCount    int          `json:"tx_count"`
	Confidence float64      `json:"confidence"`
}

type CrossChainAddr struct {
	Address string       `json:"address"`
	Chain   config.Chain `json:"chain"`
	HasTxs  bool         `json:"has_txs"`
	TxCount int          `json:"tx_count"`
}

type CoTrader struct {
	Address       string       `json:"address"`
	Chain         config.Chain `json:"chain"`
	SharedTokens  int          `json:"shared_tokens"`
	TimingOverlap float64      `json:"timing_overlap_pct"`
}

// StudyWallet performs deep analysis of a wallet and discovers all linked addresses.
// This is called immediately when a user manually adds a wallet for a KOL.
func (e *WalletStudyEngine) StudyWallet(ctx context.Context, kolID int64, address string, chain config.Chain) (*StudyResult, error) {
	result := &StudyResult{
		WalletAddress: address,
		Chain:         chain,
	}

	log.Info().Str("wallet", abbrev(address)).Str("chain", string(chain)).
		Msg("🔬 starting deep wallet study")

	// Get or create the wallet record
	wallet, err := e.store.GetWalletByAddress(address, chain)
	if err != nil {
		// wallet was just inserted, get it again
		time.Sleep(100 * time.Millisecond)
		wallet, err = e.store.GetWalletByAddress(address, chain)
		if err != nil {
			return result, fmt.Errorf("wallet not found in db: %w", err)
		}
	}

	// ── Step 1: Fetch full transaction history ──
	txCount, err := e.scanner.ScanWallet(ctx, wallet.ID, address, chain)
	if err != nil {
		log.Warn().Err(err).Str("wallet", abbrev(address)).Msg("scan error")
	}
	result.TransactionsFound = txCount
	log.Info().Str("wallet", abbrev(address)).Int("txs", txCount).Msg("📦 scanned transactions")

	// ── Step 2: Trace direct transfers (linked wallets) ──
	linked, err := e.scanner.FindLinkedWallets(ctx, address, chain, 1)
	if err == nil {
		for _, l := range linked {
			// Skip known service addresses
			if isServiceAddr(l.SourceAddress, chain) {
				continue
			}

			confidence := 0.5
			if l.SourceType == "sent_to" {
				confidence = 0.6 // direct send = higher confidence of control
			}

			result.LinkedWallets = append(result.LinkedWallets, LinkedWallet{
				Address:    l.SourceAddress,
				Chain:      chain,
				Relation:   l.SourceType,
				Amount:     l.Amount,
				TxCount:    1,
				Confidence: confidence,
			})

			// Store the linked wallet
			label := "linked:" + l.SourceType
			e.store.UpsertWallet(kolID, l.SourceAddress, chain, label, confidence,
				fmt.Sprintf("study:%s", abbrev(address)))

			log.Info().Str("found", abbrev(l.SourceAddress)).Str("relation", l.SourceType).
				Float64("amount", l.Amount).Msg("🔗 linked wallet discovered")
		}
	}

	// ── Step 3: Check funding sources ──
	funding, err := e.scanner.CheckFunding(ctx, address, chain)
	if err == nil {
		result.FundingSources = funding.FundingSources

		for _, src := range funding.FundingSources {
			if src.SourceType != "unknown" {
				log.Info().Str("source", src.SourceType).Str("from", abbrev(src.SourceAddress)).
					Float64("amount", src.Amount).Msg("💰 funding source identified")
			}
		}
	}

	// ── Step 4: Cross-chain check (EVM wallets on multiple chains) ──
	if strings.HasPrefix(address, "0x") {
		for _, otherChain := range config.AllEVMChains() {
			if otherChain == chain {
				continue
			}

			otherFunding, err := e.scanner.CheckFunding(ctx, address, otherChain)
			if err != nil || otherFunding.IsNewWallet {
				continue
			}

			cc := CrossChainAddr{
				Address: address,
				Chain:   otherChain,
				HasTxs:  true,
			}

			// Also scan this chain
			ccWallet := db.TrackedWallet{ID: 0}
			e.store.UpsertWallet(kolID, address, otherChain, "cross_chain", 0.9,
				fmt.Sprintf("cross_chain:%s", string(chain)))
			ccWallet2, _ := e.store.GetWalletByAddress(address, otherChain)
			if ccWallet2 != nil {
				ccWallet = *ccWallet2
			}

			if ccWallet.ID > 0 {
				cnt, _ := e.scanner.ScanWallet(ctx, ccWallet.ID, address, otherChain)
				cc.TxCount = cnt
			}

			result.CrossChainAddrs = append(result.CrossChainAddrs, cc)

			log.Info().Str("address", abbrev(address)).Str("chain", string(otherChain)).
				Int("txs", cc.TxCount).Msg("🌐 cross-chain wallet found")

			time.Sleep(500 * time.Millisecond) // rate limit
		}
	}

	// ── Step 5: Second-degree linked wallets ──
	// For each discovered linked wallet, check THEIR transfers too (depth 2)
	for _, lw := range result.LinkedWallets {
		if ctx.Err() != nil {
			break
		}
		if lw.Confidence < 0.5 {
			continue
		}

		secondLinked, _ := e.scanner.FindLinkedWallets(ctx, lw.Address, lw.Chain, 1)
		for _, sl := range secondLinked {
			if sl.SourceAddress == address || isServiceAddr(sl.SourceAddress, chain) {
				continue
			}

			// Lower confidence for second-degree links
			e.store.UpsertWallet(kolID, sl.SourceAddress, sl.Chain,
				"linked:2nd_degree", 0.3,
				fmt.Sprintf("via:%s", abbrev(lw.Address)))
		}

		time.Sleep(500 * time.Millisecond)
	}

	// ── Step 6: Find co-traders (wallets that bought similar tokens) ──
	txs, _ := e.store.GetTransactionsForWallet(wallet.ID, 200)
	tokens := map[string]time.Time{} // token -> first buy time
	for _, tx := range txs {
		if tx.TxType == "swap_buy" && tx.TokenAddress != "" {
			if _, ok := tokens[tx.TokenAddress]; !ok {
				tokens[tx.TokenAddress] = tx.Timestamp
			}
		}
	}

	// For each token this wallet bought, find other buyers
	if chain == config.ChainSolana && e.cfg.BirdeyeAPIKey != "" {
		for tokenAddr, buyTime := range tokens {
			if ctx.Err() != nil {
				break
			}

			buyers, _ := e.scanner.GetRecentTokenBuyers(ctx, tokenAddr, chain)
			for _, buyer := range buyers {
				if buyer.Address == address {
					continue
				}
				// Check if they bought around the same time
				timeDiff := buyer.Timestamp.Sub(buyTime).Seconds()
				if timeDiff > -3600 && timeDiff < 3600 { // within 1 hour
					result.CoTraders = append(result.CoTraders, CoTrader{
						Address:      buyer.Address,
						Chain:        chain,
						SharedTokens: 1,
					})
				}
			}

			time.Sleep(500 * time.Millisecond) // birdeye rate limit
		}
	}

	// Store alert about study completion
	e.store.InsertAlert(kolID, "wallet_study_complete", "info",
		fmt.Sprintf("Wallet study: %s found %d txs, %d linked wallets, %d cross-chain",
			abbrev(address), result.TransactionsFound, len(result.LinkedWallets), len(result.CrossChainAddrs)),
		"", address, "")

	log.Info().Str("wallet", abbrev(address)).
		Int("txs", result.TransactionsFound).
		Int("linked", len(result.LinkedWallets)).
		Int("cross_chain", len(result.CrossChainAddrs)).
		Int("co_traders", len(result.CoTraders)).
		Msg("🔬 wallet study complete")

	return result, nil
}

// StudyAllWalletsForKOL studies all known wallets for a KOL.
func (e *WalletStudyEngine) StudyAllWalletsForKOL(ctx context.Context, kolID int64) error {
	wallets, err := e.store.GetWalletsForKOL(kolID)
	if err != nil {
		return err
	}

	for _, w := range wallets {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if w.Confidence < 0.8 {
			continue // only study high-confidence wallets
		}

		_, err := e.StudyWallet(ctx, kolID, w.Address, w.Chain)
		if err != nil {
			log.Warn().Err(err).Str("wallet", abbrev(w.Address)).Msg("study error")
		}

		time.Sleep(time.Second) // rate limit between wallets
	}

	return nil
}

func isServiceAddr(addr string, chain config.Chain) bool {
	for _, a := range config.KnownFixedFloatAddresses[chain] {
		if strings.EqualFold(addr, a) {
			return true
		}
	}
	for _, a := range config.KnownBridgeContracts[chain] {
		if strings.EqualFold(addr, a) {
			return true
		}
	}
	return false
}
