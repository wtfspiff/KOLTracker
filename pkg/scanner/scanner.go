package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
)

type Scanner struct {
	cfg    *config.Config
	store  *db.Store
	client *http.Client
}

func New(cfg *config.Config, store *db.Store) *Scanner {
	return &Scanner{cfg: cfg, store: store, client: &http.Client{Timeout: 30 * time.Second}}
}

// ScanWallet dispatches to the right chain scanner.
// Prefers direct RPC (Chainstack) when available, falls back to Etherscan/Helius.
func (s *Scanner) ScanWallet(ctx context.Context, walletID int64, address string, chain config.Chain) (int, error) {
	if chain == config.ChainSolana {
		// Prefer Helius for Solana (richer parsed data), fall back to standard RPC
		if s.cfg.HeliusAPIKey != "" {
			return s.scanSolana(ctx, walletID, address)
		}
		return s.scanSolanaViaRPC(ctx, walletID, address)
	}
	// For EVM: prefer RPC for token transfers (no rate limit), Etherscan for native txs
	if s.cfg.EVMRPC[chain] != "" {
		return s.scanEVMviaRPC(ctx, walletID, address, chain)
	}
	return s.scanEVM(ctx, walletID, address, chain)
}

// CheckFunding dispatches funding analysis to the right chain.
func (s *Scanner) CheckFunding(ctx context.Context, address string, chain config.Chain) (*db.FundingAnalysis, error) {
	if chain == config.ChainSolana {
		return s.checkSolanaFunding(ctx, address)
	}
	return s.checkEVMFunding(ctx, address, chain)
}

// FindLinkedWallets traces transfers to discover connected wallets.
func (s *Scanner) FindLinkedWallets(ctx context.Context, address string, chain config.Chain, depth int) ([]db.FundingSource, error) {
	if chain == config.ChainSolana {
		return s.findSolanaLinked(ctx, address)
	}
	return s.findEVMLinked(ctx, address, chain)
}

// GetRecentTokenBuyers fetches addresses that recently bought a token.
func (s *Scanner) GetRecentTokenBuyers(ctx context.Context, tokenAddr string, chain config.Chain) ([]TokenBuyer, error) {
	if chain == config.ChainSolana && s.cfg.BirdeyeAPIKey != "" {
		return s.birdeyeBuyers(ctx, tokenAddr)
	}
	// EVM: query recent token transfer events from block explorer
	if strings.HasPrefix(tokenAddr, "0x") {
		return s.evmTokenBuyers(ctx, tokenAddr, chain)
	}
	return nil, nil
}

type TokenBuyer struct {
	Address   string       `json:"address"`
	AmountUSD float64      `json:"amount_usd"`
	TxHash    string       `json:"tx_hash"`
	Timestamp time.Time    `json:"timestamp"`
	Source    string       `json:"source"`
	Chain     config.Chain `json:"chain"`
}

// ── Solana ──────────────────────────────────────────────────

func (s *Scanner) scanSolana(ctx context.Context, walletID int64, address string) (int, error) {
	if s.cfg.HeliusAPIKey == "" {
		return 0, fmt.Errorf("helius API key required for solana scanning")
	}

	solMints := map[string]bool{
		"So11111111111111111111111111111111111111112":  true,
		"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v": true, // USDC
		"Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB":  true, // USDT
	}
	solPrice := s.getSolPrice(ctx)

	count := 0

	// Scan BOTH swap and transfer tx types for full coverage
	for _, txType := range []string{"SWAP", "TRANSFER"} {
		url := fmt.Sprintf("https://api.helius.xyz/v0/addresses/%s/transactions?api-key=%s&type=%s&limit=100",
			address, s.cfg.HeliusAPIKey, txType)

		body, err := s.getJSON(ctx, url)
		if err != nil {
			log.Warn().Err(err).Str("type", txType).Msg("helius fetch failed")
			continue
		}

		var txs []json.RawMessage
		json.Unmarshal(body, &txs)

		for _, raw := range txs {
			var p struct {
				Signature      string `json:"signature"`
				Timestamp      int64  `json:"timestamp"`
				Type           string `json:"type"`
				Source         string `json:"source"`
				Fee            int64  `json:"fee"`
				FeePayer       string `json:"feePayer"`
				TokenTransfers []struct {
					Mint            string  `json:"mint"`
					FromUserAccount string  `json:"fromUserAccount"`
					ToUserAccount   string  `json:"toUserAccount"`
					TokenAmount     float64 `json:"tokenAmount"`
				} `json:"tokenTransfers"`
				NativeTransfers []struct {
					FromUserAccount string `json:"fromUserAccount"`
					ToUserAccount   string `json:"toUserAccount"`
					Amount          int64  `json:"amount"`
				} `json:"nativeTransfers"`
			}
			if json.Unmarshal(raw, &p) != nil {
				continue
			}

			tx := db.WalletTransaction{
				WalletID:    walletID,
				TxHash:      p.Signature,
				Chain:       config.ChainSolana,
				Timestamp:   time.Unix(p.Timestamp, 0),
				Platform:    p.Source,
				PriorityFee: float64(p.Fee),
			}

			// For SWAP transactions: classify token buys/sells
			if txType == "SWAP" {
				for _, tt := range p.TokenTransfers {
					if tt.ToUserAccount == address && !solMints[tt.Mint] {
						tx.TxType = "swap_buy"
						tx.TokenAddress = tt.Mint
						tx.AmountToken = tt.TokenAmount
					} else if tt.FromUserAccount == address && !solMints[tt.Mint] {
						tx.TxType = "swap_sell"
						tx.TokenAddress = tt.Mint
						tx.AmountToken = tt.TokenAmount
					}
				}
			}

			// For TRANSFER transactions: classify SOL/SPL transfers
			if txType == "TRANSFER" {
				// SPL token transfers
				for _, tt := range p.TokenTransfers {
					if tt.ToUserAccount == address {
						tx.TxType = "transfer_in"
						tx.TokenAddress = tt.Mint
						tx.AmountToken = tt.TokenAmount
					} else if tt.FromUserAccount == address {
						tx.TxType = "transfer_out"
						tx.TokenAddress = tt.Mint
						tx.AmountToken = tt.TokenAmount
					}
				}
				// Native SOL transfers (only if no token transfer set the type)
				if tx.TxType == "" {
					for _, nt := range p.NativeTransfers {
						sol := float64(nt.Amount) / 1e9
						if sol < 0.001 {
							continue // skip dust/fees
						}
						if nt.ToUserAccount == address {
							tx.TxType = "transfer_in"
							tx.TokenSymbol = "SOL"
							tx.AmountToken = sol
							tx.FromAddress = nt.FromUserAccount
						} else if nt.FromUserAccount == address {
							tx.TxType = "transfer_out"
							tx.TokenSymbol = "SOL"
							tx.AmountToken = sol
							tx.ToAddress = nt.ToUserAccount
						}
					}
				}
			}

			// Calculate USD for native SOL transfers
			for _, nt := range p.NativeTransfers {
				sol := float64(nt.Amount) / 1e9
				if nt.FromUserAccount == address || nt.ToUserAccount == address {
					tx.AmountUSD = sol * solPrice
				}
			}

			if tx.TxType != "" {
				if s.store.InsertTransaction(tx) == nil {
					count++
				}
			}
		}
	}

	log.Info().Str("addr", abbrev(address)).Int("txs", count).Msg("scanned solana")
	return count, nil
}

func (s *Scanner) checkSolanaFunding(ctx context.Context, address string) (*db.FundingAnalysis, error) {
	fa := &db.FundingAnalysis{Address: address, Chain: config.ChainSolana, NativeSymbol: "SOL"}

	if s.cfg.HeliusAPIKey == "" {
		return fa, nil
	}

	url := fmt.Sprintf("https://api.helius.xyz/v0/addresses/%s/transactions?api-key=%s&type=TRANSFER&limit=50",
		address, s.cfg.HeliusAPIKey)

	body, err := s.getJSON(ctx, url)
	if err != nil {
		return fa, err
	}

	var txs []json.RawMessage
	json.Unmarshal(body, &txs)

	if len(txs) == 0 {
		fa.IsNewWallet = true
		return fa, nil
	}

	for _, raw := range txs {
		var p struct {
			Signature       string `json:"signature"`
			Timestamp       int64  `json:"timestamp"`
			NativeTransfers []struct {
				FromUserAccount string `json:"fromUserAccount"`
				ToUserAccount   string `json:"toUserAccount"`
				Amount          int64  `json:"amount"`
			} `json:"nativeTransfers"`
		}
		json.Unmarshal(raw, &p)

		for _, nt := range p.NativeTransfers {
			if nt.ToUserAccount == address {
				sol := float64(nt.Amount) / 1e9
				fa.TotalFunded += sol

				srcType := s.identifyAddress(ctx, nt.FromUserAccount, config.ChainSolana)
				if srcType != "unknown" {
					fa.FundingSources = append(fa.FundingSources, db.FundingSource{
						SourceAddress: nt.FromUserAccount,
						Amount:        sol,
						Token:         "SOL",
						TxHash:        p.Signature,
						SourceType:    srcType,
						Timestamp:     p.Timestamp,
						Chain:         config.ChainSolana,
					})
				}
			}
		}

		txTime := time.Unix(p.Timestamp, 0)
		if fa.FirstTxTime == nil || txTime.Before(*fa.FirstTxTime) {
			fa.FirstTxTime = &txTime
		}
	}

	return fa, nil
}

func (s *Scanner) findSolanaLinked(ctx context.Context, address string) ([]db.FundingSource, error) {
	if s.cfg.HeliusAPIKey == "" {
		return nil, nil
	}

	url := fmt.Sprintf("https://api.helius.xyz/v0/addresses/%s/transactions?api-key=%s&type=TRANSFER&limit=50",
		address, s.cfg.HeliusAPIKey)

	body, err := s.getJSON(ctx, url)
	if err != nil {
		return nil, err
	}

	var txs []json.RawMessage
	json.Unmarshal(body, &txs)

	seen := map[string]bool{address: true}
	var linked []db.FundingSource

	for _, raw := range txs {
		var p struct {
			NativeTransfers []struct {
				FromUserAccount string `json:"fromUserAccount"`
				ToUserAccount   string `json:"toUserAccount"`
				Amount          int64  `json:"amount"`
			} `json:"nativeTransfers"`
		}
		json.Unmarshal(raw, &p)

		for _, nt := range p.NativeTransfers {
			if nt.FromUserAccount == address && !seen[nt.ToUserAccount] {
				seen[nt.ToUserAccount] = true
				linked = append(linked, db.FundingSource{
					SourceAddress: nt.ToUserAccount,
					Amount:        float64(nt.Amount) / 1e9,
					Token:         "SOL",
					SourceType:    "sent_to",
					Chain:         config.ChainSolana,
				})
			}
			if nt.ToUserAccount == address && !seen[nt.FromUserAccount] {
				seen[nt.FromUserAccount] = true
				linked = append(linked, db.FundingSource{
					SourceAddress: nt.FromUserAccount,
					Amount:        float64(nt.Amount) / 1e9,
					Token:         "SOL",
					SourceType:    "received_from",
					Chain:         config.ChainSolana,
				})
			}
		}
	}

	return linked, nil
}

func (s *Scanner) birdeyeBuyers(ctx context.Context, tokenAddr string) ([]TokenBuyer, error) {
	url := fmt.Sprintf("https://public-api.birdeye.so/defi/txs/token?address=%s&tx_type=swap&sort_type=desc&limit=50", tokenAddr)

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("X-API-KEY", s.cfg.BirdeyeAPIKey)
	req.Header.Set("x-chain", "solana")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Items []struct {
				Owner     string  `json:"owner"`
				VolumeUSD float64 `json:"volume_usd"`
				TxHash    string  `json:"tx_hash"`
				BlockTime int64   `json:"block_unix_time"`
				Side      string  `json:"side"`
				Source    string  `json:"source"`
			} `json:"items"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	var buyers []TokenBuyer
	for _, item := range result.Data.Items {
		if item.Side == "buy" {
			buyers = append(buyers, TokenBuyer{
				Address:   item.Owner,
				AmountUSD: item.VolumeUSD,
				TxHash:    item.TxHash,
				Timestamp: time.Unix(item.BlockTime, 0),
				Source:    item.Source,
				Chain:     config.ChainSolana,
			})
		}
	}
	return buyers, nil
}

// ── EVM (ETH / Base / BSC) ─────────────────────────────────

func (s *Scanner) scanEVM(ctx context.Context, walletID int64, address string, chain config.Chain) (int, error) {
	apiURL := s.cfg.GetExplorerURL(chain)
	apiKey := s.cfg.GetExplorerKey(chain)
	if apiURL == "" || apiKey == "" {
		return 0, fmt.Errorf("no explorer config for %s", chain)
	}

	native := nativeSymbol(chain)
	nativePrice := s.getNativePrice(ctx, chain)
	count := 0

	// Normal txs — extract native transfers + detect DEX interactions
	txs, _ := s.etherscanList(ctx, apiURL, apiKey, address, "txlist")
	for _, etx := range txs {
		hash := str(etx, "hash")
		if hash == "" {
			continue
		}
		from := str(etx, "from")
		to := str(etx, "to")
		value := weiToEth(str(etx, "value"))
		ts := parseUnixStr(str(etx, "timeStamp"))
		gasPrice := weiToEth(str(etx, "gasPrice"))     // in ETH
		gasUsed := parseFloat(str(etx, "gasUsed"))

		txType := "transfer_out"
		if strings.EqualFold(to, address) {
			txType = "transfer_in"
		}

		// Detect DEX router interactions (swap via native ETH/BNB)
		platform := ""
		if dex := config.ClassifyEVMDEX(to); dex != "" {
			platform = dex
			if strings.EqualFold(from, address) && value > 0 {
				txType = "swap_buy" // sending ETH to DEX = buying tokens
			}
		} else if dex := config.ClassifyEVMDEX(from); dex != "" {
			platform = dex
			if strings.EqualFold(to, address) && value > 0 {
				txType = "swap_sell" // receiving ETH from DEX = sold tokens
			}
		}

		// Extract priority fee (gas tip) — EIP-1559: gasPrice includes base+tip
		// We store total gas cost as the fee fingerprint (used for bot detection)
		priorityFee := gasPrice * gasUsed // total gas cost in ETH

		if s.store.InsertTransaction(db.WalletTransaction{
			WalletID: walletID, TxHash: hash, Chain: chain, TxType: txType,
			TokenSymbol: native, AmountToken: value, AmountUSD: value * nativePrice,
			FromAddress: from, ToAddress: to, Timestamp: ts,
			BlockNumber: parseInt64(str(etx, "blockNumber")),
			Platform:    platform,
			PriorityFee: priorityFee,
		}) == nil {
			count++
		}
	}

	// ERC-20 token transfers — detect swaps and extract DEX info
	tokenTxs, _ := s.etherscanList(ctx, apiURL, apiKey, address, "tokentx")
	stables := map[string]bool{
		"USDC": true, "USDT": true, "BUSD": true, "DAI": true,
		"WETH": true, "WBNB": true, "UST": true, "FRAX": true,
	}

	for _, etx := range tokenTxs {
		hash := str(etx, "hash")
		if hash == "" {
			continue
		}
		from := str(etx, "from")
		to := str(etx, "to")
		symbol := str(etx, "tokenSymbol")
		decimals := int(parseInt64(str(etx, "tokenDecimal")))
		if decimals == 0 {
			decimals = 18
		}
		value := tokenValue(str(etx, "value"), decimals)

		txType := "transfer_in"
		if strings.EqualFold(from, address) {
			txType = "transfer_out"
			if stables[symbol] {
				txType = "swap_buy" // sending stables = buying tokens
			}
		} else if strings.EqualFold(to, address) && stables[symbol] {
			txType = "swap_sell" // receiving stables = sold tokens
		}

		// For stablecoins, AmountUSD ≈ AmountToken
		amountUSD := 0.0
		if stables[symbol] {
			amountUSD = value
		}

		// Try to detect DEX from "from" or "to" in token transfer context
		platform := ""
		// In ERC-20 transfers, the "from"/"to" might not be the DEX router itself
		// But we can check if the *other* participant is a known router
		counterparty := to
		if strings.EqualFold(to, address) {
			counterparty = from
		}
		if dex := config.ClassifyEVMDEX(counterparty); dex != "" {
			platform = dex
		}

		if s.store.InsertTransaction(db.WalletTransaction{
			WalletID: walletID, TxHash: hash, Chain: chain, TxType: txType,
			TokenAddress: str(etx, "contractAddress"), TokenSymbol: symbol,
			AmountToken: value, AmountUSD: amountUSD, FromAddress: from, ToAddress: to,
			Timestamp: parseUnixStr(str(etx, "timeStamp")),
			Platform: platform,
		}) == nil {
			count++
		}
	}

	// Internal transactions — catches ETH received from DEX swaps
	// (DEX routers often send ETH via internal calls, not direct transfers)
	internalTxs, _ := s.etherscanList(ctx, apiURL, apiKey, address, "txlistinternal")
	for _, etx := range internalTxs {
		hash := str(etx, "hash")
		if hash == "" {
			continue
		}
		from := str(etx, "from")
		to := str(etx, "to")
		value := weiToEth(str(etx, "value"))
		if value == 0 {
			continue
		}

		txType := "transfer_in"
		platform := ""
		if strings.EqualFold(to, address) {
			// Receiving ETH via internal tx — check if from a DEX router
			if dex := config.ClassifyEVMDEX(from); dex != "" {
				txType = "swap_sell" // DEX router sent us ETH = we sold tokens
				platform = dex
			}
		} else if strings.EqualFold(from, address) {
			txType = "transfer_out"
		}

		if s.store.InsertTransaction(db.WalletTransaction{
			WalletID: walletID, TxHash: hash, Chain: chain, TxType: txType,
			TokenSymbol: native, AmountToken: value, AmountUSD: value * nativePrice,
			FromAddress: from, ToAddress: to,
			Timestamp: parseUnixStr(str(etx, "timeStamp")),
			Platform:  platform,
		}) == nil {
			count++
		}
	}

	log.Info().Str("addr", abbrev(address)).Str("chain", string(chain)).Int("txs", count).Msg("scanned EVM")
	return count, nil
}

func (s *Scanner) checkEVMFunding(ctx context.Context, address string, chain config.Chain) (*db.FundingAnalysis, error) {
	native := nativeSymbol(chain)
	fa := &db.FundingAnalysis{Address: address, Chain: chain, NativeSymbol: native}

	apiURL := s.cfg.GetExplorerURL(chain)
	apiKey := s.cfg.GetExplorerKey(chain)
	if apiURL == "" || apiKey == "" {
		return fa, nil
	}

	txs, err := s.etherscanList(ctx, apiURL, apiKey, address, "txlist")
	if err != nil || len(txs) == 0 {
		fa.IsNewWallet = true
		return fa, err
	}

	for _, etx := range txs {
		to := str(etx, "to")
		if !strings.EqualFold(to, address) {
			continue
		}

		from := str(etx, "from")
		value := weiToEth(str(etx, "value"))
		fa.TotalFunded += value

		srcType := s.identifyAddress(ctx, from, chain)
		if srcType != "unknown" {
			fa.FundingSources = append(fa.FundingSources, db.FundingSource{
				SourceAddress: from, Amount: value, Token: native,
				TxHash: str(etx, "hash"), SourceType: srcType,
				Timestamp: parseInt64(str(etx, "timeStamp")), Chain: chain,
			})
			log.Warn().Str("wallet", abbrev(address)).Str("chain", string(chain)).
				Str("source", srcType).Float64("amount", value).Msg("🚨 suspicious EVM funding")
		}

		txTime := parseUnixStr(str(etx, "timeStamp"))
		if !txTime.IsZero() && (fa.FirstTxTime == nil || txTime.Before(*fa.FirstTxTime)) {
			fa.FirstTxTime = &txTime
		}
	}

	return fa, nil
}

func (s *Scanner) findEVMLinked(ctx context.Context, address string, chain config.Chain) ([]db.FundingSource, error) {
	apiURL := s.cfg.GetExplorerURL(chain)
	apiKey := s.cfg.GetExplorerKey(chain)
	if apiURL == "" {
		return nil, nil
	}

	txs, _ := s.etherscanList(ctx, apiURL, apiKey, address, "txlist")
	seen := map[string]bool{strings.ToLower(address): true}
	var linked []db.FundingSource

	for _, etx := range txs {
		from := strings.ToLower(str(etx, "from"))
		to := strings.ToLower(str(etx, "to"))
		value := weiToEth(str(etx, "value"))

		if from == strings.ToLower(address) && !seen[to] {
			seen[to] = true
			linked = append(linked, db.FundingSource{
				SourceAddress: to, Amount: value, Token: nativeSymbol(chain),
				SourceType: "sent_to", Chain: chain,
			})
		}
		if to == strings.ToLower(address) && !seen[from] {
			seen[from] = true
			linked = append(linked, db.FundingSource{
				SourceAddress: from, Amount: value, Token: nativeSymbol(chain),
				SourceType: "received_from", Chain: chain,
			})
		}
	}

	return linked, nil
}

// ── Shared helpers ──────────────────────────────────────────

func (s *Scanner) identifyAddress(ctx context.Context, address string, chain config.Chain) string {
	for _, addr := range config.KnownFixedFloatAddresses[chain] {
		if strings.EqualFold(address, addr) {
			return "fixedfloat"
		}
	}
	for _, addr := range config.KnownBridgeContracts[chain] {
		if strings.EqualFold(address, addr) {
			return "bridge"
		}
	}

	// Check known DEX routers / CEX hot wallets
	if label := config.IdentifyKnownEVMAddress(address); label != "" {
		return label
	}

	// Solscan label check (Solana)
	if chain == config.ChainSolana && s.cfg.SolscanAPIKey != "" {
		url := fmt.Sprintf("https://pro-api.solscan.io/v2.0/account/%s", address)
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("token", s.cfg.SolscanAPIKey)
		if resp, err := s.client.Do(req); err == nil {
			defer resp.Body.Close()
			var data struct {
				Data struct{ Label string `json:"account_label"` } `json:"data"`
			}
			json.NewDecoder(resp.Body).Decode(&data)
			if data.Data.Label != "" {
				return matchServiceLabel(data.Data.Label)
			}
		}
	}

	// EVM contract detection — prefer RPC, fallback to Etherscan
	if strings.HasPrefix(address, "0x") && chain != config.ChainSolana {
		// Try RPC first (faster, no rate limit)
		if rpcURL := s.cfg.EVMRPC[chain]; rpcURL != "" {
			if s.isContract(ctx, rpcURL, address) {
				return "contract"
			}
			return "unknown"
		}

		// Etherscan fallback
		apiURL := s.cfg.GetExplorerURL(chain)
		apiKey := s.cfg.GetExplorerKey(chain)
		if apiURL != "" && apiKey != "" {
			url := fmt.Sprintf("%s?module=contract&action=getabi&address=%s&apikey=%s",
				apiURL, address, apiKey)
			body, err := s.getJSON(ctx, url)
			if err == nil {
				var result struct {
					Status string `json:"status"`
					Result string `json:"result"`
				}
				json.Unmarshal(body, &result)
				if result.Status == "1" && result.Result != "Contract source code not verified" {
					return "contract"
				}
			}
		}
	}

	return "unknown"
}

type etherscanResult = map[string]interface{}

func (s *Scanner) etherscanList(ctx context.Context, apiURL, apiKey, address, action string) ([]etherscanResult, error) {
	url := fmt.Sprintf("%s?module=account&action=%s&address=%s&startblock=0&endblock=99999999&page=1&offset=100&sort=desc&apikey=%s",
		apiURL, action, address, apiKey)

	body, err := s.getJSON(ctx, url)
	if err != nil {
		return nil, err
	}

	var result struct {
		Status string            `json:"status"`
		Result []etherscanResult `json:"result"`
	}
	json.Unmarshal(body, &result)

	if result.Status != "1" {
		return nil, fmt.Errorf("etherscan status: %s", result.Status)
	}
	return result.Result, nil
}

func (s *Scanner) getJSON(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB max
}

func str(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func nativeSymbol(chain config.Chain) string {
	if chain == config.ChainBSC {
		return "BNB"
	}
	return "ETH"
}

// ── Live Price Fetching ─────────────────────────────────────

var (
	priceCache     = map[string]cachedPrice{}
	priceCacheLock sync.RWMutex
)

type cachedPrice struct {
	price   float64
	fetched time.Time
}

// getSolPrice returns the current SOL/USD price via DexScreener (free, no API key).
// Caches for 60 seconds to avoid rate limits.
func (s *Scanner) getSolPrice(ctx context.Context) float64 {
	return s.getTokenPrice(ctx, "solana", "So11111111111111111111111111111111111111112")
}

// getETHPrice returns the current ETH/USD price.
func (s *Scanner) getETHPrice(ctx context.Context) float64 {
	return s.getTokenPrice(ctx, "ethereum", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
}

// getBNBPrice returns the current BNB/USD price.
func (s *Scanner) getBNBPrice(ctx context.Context) float64 {
	return s.getTokenPrice(ctx, "bsc", "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c")
}

// getNativePrice returns the native token price for any chain.
func (s *Scanner) getNativePrice(ctx context.Context, chain config.Chain) float64 {
	switch chain {
	case config.ChainSolana:
		return s.getSolPrice(ctx)
	case config.ChainBSC:
		return s.getBNBPrice(ctx)
	default: // ETH, Base (uses ETH)
		return s.getETHPrice(ctx)
	}
}

// getTokenPrice fetches from DexScreener with 60s cache.
func (s *Scanner) getTokenPrice(ctx context.Context, dexChain, tokenAddr string) float64 {
	cacheKey := dexChain + ":" + tokenAddr

	priceCacheLock.RLock()
	if c, ok := priceCache[cacheKey]; ok && time.Since(c.fetched) < 60*time.Second {
		priceCacheLock.RUnlock()
		return c.price
	}
	priceCacheLock.RUnlock()

	url := fmt.Sprintf("https://api.dexscreener.com/latest/dex/tokens/%s", tokenAddr)
	body, err := s.getJSON(ctx, url)
	if err != nil {
		return fallbackPrice(dexChain)
	}

	var result struct {
		Pairs []struct {
			PriceUSD    string `json:"priceUsd"`
			ChainID     string `json:"chainId"`
			Liquidity   struct{ USD float64 `json:"usd"` } `json:"liquidity"`
		} `json:"pairs"`
	}
	if json.Unmarshal(body, &result) != nil || len(result.Pairs) == 0 {
		return fallbackPrice(dexChain)
	}

	// Pick highest liquidity pair
	bestPrice := 0.0
	bestLiq := 0.0
	for _, p := range result.Pairs {
		if price := parseFloat(p.PriceUSD); price > 0 && p.Liquidity.USD > bestLiq {
			bestPrice = price
			bestLiq = p.Liquidity.USD
		}
	}

	if bestPrice > 0 {
		priceCacheLock.Lock()
		priceCache[cacheKey] = cachedPrice{price: bestPrice, fetched: time.Now()}
		priceCacheLock.Unlock()
		return bestPrice
	}

	return fallbackPrice(dexChain)
}

func fallbackPrice(chain string) float64 {
	// Conservative fallback prices if API is down
	switch chain {
	case "solana":
		return 150.0
	case "ethereum":
		return 2500.0
	case "bsc":
		return 300.0
	default:
		return 2500.0
	}
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// ── EVM Token Buyers ────────────────────────────────────────

// evmTokenBuyers fetches recent buyers of an ERC-20 token using Etherscan/Basescan token transfer API.
// Enriches with USD amounts by looking up token price from DexScreener.
func (s *Scanner) evmTokenBuyers(ctx context.Context, tokenAddr string, chain config.Chain) ([]TokenBuyer, error) {
	apiURL := s.cfg.GetExplorerURL(chain)
	apiKey := s.cfg.GetExplorerKey(chain)
	if apiURL == "" || apiKey == "" {
		return nil, fmt.Errorf("no explorer config for %s", chain)
	}

	// Fetch recent token transfers for this contract
	url := fmt.Sprintf("%s?module=account&action=tokentx&contractaddress=%s&page=1&offset=100&sort=desc&apikey=%s",
		apiURL, tokenAddr, apiKey)

	body, err := s.getJSON(ctx, url)
	if err != nil {
		return nil, err
	}

	var result struct {
		Status string            `json:"status"`
		Result []etherscanResult `json:"result"`
	}
	json.Unmarshal(body, &result)
	if result.Status != "1" {
		return nil, fmt.Errorf("explorer status: %s", result.Status)
	}

	// Look up token price once for all buyers (DexScreener, cached 60s)
	dexChain := "ethereum"
	switch chain {
	case config.ChainBSC:
		dexChain = "bsc"
	case config.ChainBase:
		dexChain = "base"
	}
	tokenPrice := s.getTokenPrice(ctx, dexChain, tokenAddr)

	// Deduplicate buyers (wallets that received the token)
	seen := map[string]bool{}
	var buyers []TokenBuyer

	for _, tx := range result.Result {
		to := str(tx, "to")
		from := str(tx, "from")
		if to == "" || seen[strings.ToLower(to)] {
			continue
		}

		// Skip known DEX routers and contracts — we want end-user wallets
		if config.ClassifyEVMDEX(to) != "" {
			continue
		}

		decimals := int(parseInt64(str(tx, "tokenDecimal")))
		if decimals == 0 {
			decimals = 18
		}
		value := tokenValue(str(tx, "value"), decimals)
		ts := parseUnixStr(str(tx, "timeStamp"))

		// Calculate USD amount using token price
		amountUSD := value * tokenPrice

		seen[strings.ToLower(to)] = true
		buyers = append(buyers, TokenBuyer{
			Address:   to,
			AmountUSD: amountUSD,
			TxHash:    str(tx, "hash"),
			Timestamp: ts,
			Source:    fmt.Sprintf("from:%s", abbrev(from)),
			Chain:     chain,
		})
	}

	return buyers, nil
}
