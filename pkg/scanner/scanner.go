package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
func (s *Scanner) ScanWallet(ctx context.Context, walletID int64, address string, chain config.Chain) (int, error) {
	if chain == config.ChainSolana {
		return s.scanSolana(ctx, walletID, address)
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
	// EVM and fallback: use DexScreener (limited - no individual buyers)
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

	url := fmt.Sprintf("https://api.helius.xyz/v0/addresses/%s/transactions?api-key=%s&type=SWAP&limit=100",
		address, s.cfg.HeliusAPIKey)

	body, err := s.getJSON(ctx, url)
	if err != nil {
		return 0, err
	}

	var txs []json.RawMessage
	json.Unmarshal(body, &txs)

	solMints := map[string]bool{
		"So11111111111111111111111111111111111111112":  true,
		"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v": true,
		"Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB":  true,
	}

	count := 0
	for _, raw := range txs {
		var p struct {
			Signature      string `json:"signature"`
			Timestamp      int64  `json:"timestamp"`
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

		for _, nt := range p.NativeTransfers {
			sol := float64(nt.Amount) / 1e9
			if nt.FromUserAccount == address || nt.ToUserAccount == address {
				tx.AmountUSD = sol * 150 // TODO: live SOL price
			}
		}

		if tx.TxType != "" {
			if s.store.InsertTransaction(tx) == nil {
				count++
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
	count := 0

	// Normal txs
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

		txType := "transfer_out"
		if strings.EqualFold(to, address) {
			txType = "transfer_in"
		}

		if s.store.InsertTransaction(db.WalletTransaction{
			WalletID: walletID, TxHash: hash, Chain: chain, TxType: txType,
			TokenSymbol: native, AmountToken: value,
			FromAddress: from, ToAddress: to, Timestamp: ts,
			BlockNumber: parseInt64(str(etx, "blockNumber")),
		}) == nil {
			count++
		}
	}

	// ERC-20 token transfers
	tokenTxs, _ := s.etherscanList(ctx, apiURL, apiKey, address, "tokentx")
	stables := map[string]bool{"USDC": true, "USDT": true, "BUSD": true, "DAI": true, "WETH": true, "WBNB": true}

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
				txType = "swap_buy"
			}
		} else if strings.EqualFold(to, address) && stables[symbol] {
			txType = "swap_sell"
		}

		if s.store.InsertTransaction(db.WalletTransaction{
			WalletID: walletID, TxHash: hash, Chain: chain, TxType: txType,
			TokenAddress: str(etx, "contractAddress"), TokenSymbol: symbol,
			AmountToken: value, FromAddress: from, ToAddress: to,
			Timestamp: parseUnixStr(str(etx, "timeStamp")),
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

	// Solscan label check
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

	var buf [1 << 20]byte // 1MB max
	n, _ := resp.Body.Read(buf[:])
	return buf[:n], nil
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
