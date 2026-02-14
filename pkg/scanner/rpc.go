package scanner

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
)

// ── EVM JSON-RPC Client ─────────────────────────────────────
// Talks directly to Chainstack (or any EVM node) via JSON-RPC.
// Replaces Etherscan API for token transfers, internal txs, and contract checks.

// ERC-20 Transfer event topic: keccak256("Transfer(address,address,uint256)")
const erc20TransferTopic = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
	ID      int             `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcCall performs a JSON-RPC call to the given EVM RPC endpoint.
func (s *Scanner) rpcCall(ctx context.Context, rpcURL, method string, params []interface{}) (json.RawMessage, error) {
	reqBody, _ := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("rpc unmarshal: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

// ── eth_getLogs: ERC-20 Token Transfers ─────────────────────
// Replaces Etherscan tokentx. Fetches all ERC-20 Transfer events where
// the address is sender (topic2) or receiver (topic3).

type evmLog struct {
	Address     string   `json:"address"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
	BlockNumber string   `json:"blockNumber"`
	TxHash      string   `json:"transactionHash"`
	LogIndex    string   `json:"logIndex"`
}

// getERC20Transfers fetches ERC-20 Transfer events for an address using eth_getLogs.
// Returns transfers where address is either sender or receiver.
func (s *Scanner) getERC20Transfers(ctx context.Context, rpcURL, address string, fromBlock, toBlock string) ([]evmLog, error) {
	addr := strings.ToLower(address)
	paddedAddr := "0x000000000000000000000000" + addr[2:] // pad to 32 bytes for topic filter

	var allLogs []evmLog

	// Transfers TO this address (address is topic2 = receiver)
	filterTo := map[string]interface{}{
		"fromBlock": fromBlock,
		"toBlock":   toBlock,
		"topics":    []interface{}{erc20TransferTopic, nil, paddedAddr},
	}
	resultTo, err := s.rpcCall(ctx, rpcURL, "eth_getLogs", []interface{}{filterTo})
	if err == nil {
		var logs []evmLog
		json.Unmarshal(resultTo, &logs)
		allLogs = append(allLogs, logs...)
	}

	// Transfers FROM this address (address is topic1 = sender)
	filterFrom := map[string]interface{}{
		"fromBlock": fromBlock,
		"toBlock":   toBlock,
		"topics":    []interface{}{erc20TransferTopic, paddedAddr},
	}
	resultFrom, err := s.rpcCall(ctx, rpcURL, "eth_getLogs", []interface{}{filterFrom})
	if err == nil {
		var logs []evmLog
		json.Unmarshal(resultFrom, &logs)
		allLogs = append(allLogs, logs...)
	}

	return allLogs, nil
}

// parseERC20Log extracts from, to, amount from an ERC-20 Transfer log entry.
func parseERC20Log(l evmLog) (from, to string, amount *big.Int, ok bool) {
	if len(l.Topics) < 3 || len(l.Data) < 66 {
		return "", "", nil, false
	}
	from = "0x" + l.Topics[1][26:] // last 20 bytes of topic
	to = "0x" + l.Topics[2][26:]
	amount = new(big.Int)
	dataBytes, err := hex.DecodeString(strings.TrimPrefix(l.Data, "0x"))
	if err != nil || len(dataBytes) < 32 {
		return "", "", nil, false
	}
	amount.SetBytes(dataBytes[:32])
	return from, to, amount, true
}

// ── eth_getCode: Contract Detection ─────────────────────────
// Replaces Etherscan contract ABI check.

func (s *Scanner) isContract(ctx context.Context, rpcURL, address string) bool {
	result, err := s.rpcCall(ctx, rpcURL, "eth_getCode", []interface{}{address, "latest"})
	if err != nil {
		return false
	}
	var code string
	json.Unmarshal(result, &code)
	return code != "0x" && code != "0x0" && len(code) > 4
}

// ── eth_getBalance: Native balance ──────────────────────────

func (s *Scanner) getBalance(ctx context.Context, rpcURL, address string) float64 {
	result, err := s.rpcCall(ctx, rpcURL, "eth_getBalance", []interface{}{address, "latest"})
	if err != nil {
		return 0
	}
	var hexBal string
	json.Unmarshal(result, &hexBal)
	bal := new(big.Int)
	bal.SetString(strings.TrimPrefix(hexBal, "0x"), 16)
	f := new(big.Float).SetInt(bal)
	f.Quo(f, big.NewFloat(1e18))
	v, _ := f.Float64()
	return v
}

// ── eth_blockNumber: Current block ──────────────────────────

func (s *Scanner) getBlockNumber(ctx context.Context, rpcURL string) (int64, error) {
	result, err := s.rpcCall(ctx, rpcURL, "eth_blockNumber", []interface{}{})
	if err != nil {
		return 0, err
	}
	var hexBlock string
	json.Unmarshal(result, &hexBlock)
	block := new(big.Int)
	block.SetString(strings.TrimPrefix(hexBlock, "0x"), 16)
	return block.Int64(), nil
}

// ── eth_getBlockByNumber: Block timestamp ───────────────────

func (s *Scanner) getBlockTimestamp(ctx context.Context, rpcURL string, blockHex string) time.Time {
	result, err := s.rpcCall(ctx, rpcURL, "eth_getBlockByNumber", []interface{}{blockHex, false})
	if err != nil {
		return time.Time{}
	}
	var block struct {
		Timestamp string `json:"timestamp"`
	}
	json.Unmarshal(result, &block)
	ts := new(big.Int)
	ts.SetString(strings.TrimPrefix(block.Timestamp, "0x"), 16)
	return time.Unix(ts.Int64(), 0)
}

// ── RPC-based EVM Scanner ───────────────────────────────────
// Uses JSON-RPC directly instead of Etherscan API.

// scanEVMviaRPC scans an EVM wallet using direct JSON-RPC calls to Chainstack.
// Falls back to Etherscan if RPC URL is not configured.
func (s *Scanner) scanEVMviaRPC(ctx context.Context, walletID int64, address string, chain config.Chain) (int, error) {
	rpcURL := s.cfg.EVMRPC[chain]
	if rpcURL == "" {
		// No RPC configured — fall back to Etherscan
		return s.scanEVM(ctx, walletID, address, chain)
	}

	native := nativeSymbol(chain)
	nativePrice := s.getNativePrice(ctx, chain)
	count := 0

	// Get current block to set scan range
	currentBlock, err := s.getBlockNumber(ctx, rpcURL)
	if err != nil {
		log.Warn().Err(err).Str("chain", string(chain)).Msg("RPC getBlockNumber failed, falling back to Etherscan")
		return s.scanEVM(ctx, walletID, address, chain)
	}

	// Scan last ~50K blocks (~7 days on ETH, ~2 days on BSC, ~3 days on Base)
	fromBlock := currentBlock - 50000
	if fromBlock < 0 {
		fromBlock = 0
	}
	fromHex := fmt.Sprintf("0x%x", fromBlock)
	toHex := "latest"

	// 1. ERC-20 Token Transfers via eth_getLogs
	logs, err := s.getERC20Transfers(ctx, rpcURL, address, fromHex, toHex)
	if err != nil {
		log.Warn().Err(err).Msg("eth_getLogs failed")
	}

	stables := map[string]bool{
		"USDC": true, "USDT": true, "BUSD": true, "DAI": true,
		"WETH": true, "WBNB": true, "FRAX": true,
	}

	// Track token addresses to get symbol/decimals
	tokenInfoCache := map[string]tokenInfo{}

	for _, l := range logs {
		from, to, amount, ok := parseERC20Log(l)
		if !ok || amount.Sign() == 0 {
			continue
		}

		tokenAddr := strings.ToLower(l.Address)
		info := s.getTokenInfo(ctx, rpcURL, tokenAddr, tokenInfoCache)

		value := tokenValueBig(amount, info.decimals)

		txType := "transfer_in"
		if strings.EqualFold(from, address) {
			txType = "transfer_out"
			if stables[info.symbol] {
				txType = "swap_buy"
			}
		} else if strings.EqualFold(to, address) && stables[info.symbol] {
			txType = "swap_sell"
		}

		amountUSD := 0.0
		if stables[info.symbol] {
			amountUSD = value
		}

		// DEX detection
		counterparty := to
		if strings.EqualFold(to, address) {
			counterparty = from
		}
		platform := ""
		if dex := config.ClassifyEVMDEX(counterparty); dex != "" {
			platform = dex
		}

		ts := s.getBlockTimestamp(ctx, rpcURL, l.BlockNumber)

		if s.store.InsertTransaction(db.WalletTransaction{
			WalletID:     walletID,
			TxHash:       l.TxHash,
			Chain:        chain,
			TxType:       txType,
			TokenAddress: tokenAddr,
			TokenSymbol:  info.symbol,
			AmountToken:  value,
			AmountUSD:    amountUSD,
			FromAddress:  from,
			ToAddress:    to,
			Timestamp:    ts,
			Platform:     platform,
		}) == nil {
			count++
		}
	}

	// 2. Native transfers — we still need Etherscan/trace for this (no eth_getLogs for native)
	// Use Etherscan txlist as fallback for native ETH/BNB transfers
	apiURL := s.cfg.GetExplorerURL(chain)
	apiKey := s.cfg.GetExplorerKey(chain)
	if apiURL != "" && apiKey != "" {
		txs, _ := s.etherscanList(ctx, apiURL, apiKey, address, "txlist")
		for _, etx := range txs {
			hash := str(etx, "hash")
			if hash == "" {
				continue
			}
			from := str(etx, "from")
			to := str(etx, "to")
			value := weiToEth(str(etx, "value"))
			if value == 0 {
				continue // skip zero-value contract calls (already covered by getLogs)
			}
			ts := parseUnixStr(str(etx, "timeStamp"))
			gasPrice := weiToEth(str(etx, "gasPrice"))
			gasUsed := parseFloat(str(etx, "gasUsed"))

			txType := "transfer_out"
			if strings.EqualFold(to, address) {
				txType = "transfer_in"
			}
			platform := ""
			if dex := config.ClassifyEVMDEX(to); dex != "" {
				platform = dex
				if strings.EqualFold(from, address) {
					txType = "swap_buy"
				}
			}

			if s.store.InsertTransaction(db.WalletTransaction{
				WalletID: walletID, TxHash: hash, Chain: chain, TxType: txType,
				TokenSymbol: native, AmountToken: value, AmountUSD: value * nativePrice,
				FromAddress: from, ToAddress: to, Timestamp: ts,
				BlockNumber: parseInt64(str(etx, "blockNumber")),
				Platform:    platform,
				PriorityFee: gasPrice * gasUsed,
			}) == nil {
				count++
			}
		}
	}

	log.Info().Str("addr", abbrev(address)).Str("chain", string(chain)).
		Int("txs", count).Str("mode", "rpc+etherscan").Msg("scanned EVM")
	return count, nil
}

// ── Token Info via RPC ──────────────────────────────────────

type tokenInfo struct {
	symbol   string
	decimals int
}

// getTokenInfo fetches symbol and decimals for an ERC-20 token via RPC.
func (s *Scanner) getTokenInfo(ctx context.Context, rpcURL, tokenAddr string, cache map[string]tokenInfo) tokenInfo {
	if info, ok := cache[tokenAddr]; ok {
		return info
	}

	info := tokenInfo{symbol: "UNKNOWN", decimals: 18}

	// symbol() → 0x95d89b41
	symbolResult, err := s.rpcCall(ctx, rpcURL, "eth_call", []interface{}{
		map[string]string{"to": tokenAddr, "data": "0x95d89b41"},
		"latest",
	})
	if err == nil {
		var hexData string
		json.Unmarshal(symbolResult, &hexData)
		if sym := decodeStringResult(hexData); sym != "" {
			info.symbol = sym
		}
	}

	// decimals() → 0x313ce567
	decResult, err := s.rpcCall(ctx, rpcURL, "eth_call", []interface{}{
		map[string]string{"to": tokenAddr, "data": "0x313ce567"},
		"latest",
	})
	if err == nil {
		var hexData string
		json.Unmarshal(decResult, &hexData)
		if d := decodeUintResult(hexData); d > 0 && d <= 36 {
			info.decimals = d
		}
	}

	cache[tokenAddr] = info
	return info
}

// decodeStringResult parses an ABI-encoded string from an eth_call result.
func decodeStringResult(hexData string) string {
	hexData = strings.TrimPrefix(hexData, "0x")
	if len(hexData) < 128 {
		// Could be a non-standard return (bytes32 string)
		if raw, err := hex.DecodeString(hexData); err == nil {
			// Trim null bytes
			end := len(raw)
			for end > 0 && raw[end-1] == 0 {
				end--
			}
			if end > 0 {
				s := string(raw[:end])
				// Only return printable ASCII
				for _, c := range s {
					if c < 32 || c > 126 {
						return ""
					}
				}
				return s
			}
		}
		return ""
	}

	data, err := hex.DecodeString(hexData)
	if err != nil || len(data) < 64 {
		return ""
	}

	// Standard ABI encoding: offset (32 bytes) + length (32 bytes) + data
	offset := new(big.Int).SetBytes(data[:32]).Int64()
	if offset+32 > int64(len(data)) {
		return ""
	}
	length := new(big.Int).SetBytes(data[offset : offset+32]).Int64()
	if offset+32+length > int64(len(data)) || length > 100 {
		return ""
	}
	return string(data[offset+32 : offset+32+length])
}

// decodeUintResult parses an ABI-encoded uint256 from an eth_call result.
func decodeUintResult(hexData string) int {
	hexData = strings.TrimPrefix(hexData, "0x")
	if len(hexData) < 2 {
		return 0
	}
	val := new(big.Int)
	val.SetString(hexData, 16)
	return int(val.Int64())
}

// tokenValueBig converts a big.Int token amount with given decimals to float64.
func tokenValueBig(amount *big.Int, decimals int) float64 {
	f := new(big.Float).SetInt(amount)
	div := new(big.Float).SetFloat64(1)
	for i := 0; i < decimals; i++ {
		div.Mul(div, new(big.Float).SetFloat64(10))
	}
	f.Quo(f, div)
	v, _ := f.Float64()
	return v
}

// ── Solana RPC (Chainstack compatible) ──────────────────────
// Uses getSignaturesForAddress + getTransaction for direct Solana RPC scanning
// as alternative to Helius parsed transactions API.

// scanSolanaViaRPC scans a Solana wallet using standard JSON-RPC
// (works with any RPC: Chainstack, QuickNode, Alchemy, public).
// Falls back to Helius if available for richer parsed data.
func (s *Scanner) scanSolanaViaRPC(ctx context.Context, walletID int64, address string) (int, error) {
	// If Helius is available, prefer it — it gives parsed swap data (DEX name, token transfers)
	// that standard RPC doesn't provide without manual instruction parsing
	if s.cfg.HeliusAPIKey != "" {
		return s.scanSolana(ctx, walletID, address)
	}

	rpcURL := s.cfg.SolanaRPCURL
	if rpcURL == "" {
		return 0, fmt.Errorf("no Solana RPC configured")
	}

	// getSignaturesForAddress — get recent tx signatures
	result, err := s.rpcCall(ctx, rpcURL, "getSignaturesForAddress", []interface{}{
		address,
		map[string]interface{}{"limit": 100},
	})
	if err != nil {
		return 0, fmt.Errorf("getSignaturesForAddress: %w", err)
	}

	var sigs []struct {
		Signature string `json:"signature"`
		Slot      int64  `json:"slot"`
		BlockTime *int64 `json:"blockTime"`
		Err       interface{} `json:"err"`
	}
	json.Unmarshal(result, &sigs)

	solPrice := s.getSolPrice(ctx)
	count := 0

	for _, sig := range sigs {
		if sig.Err != nil {
			continue // skip failed txs
		}

		// getTransaction with maxSupportedTransactionVersion
		txResult, err := s.rpcCall(ctx, rpcURL, "getTransaction", []interface{}{
			sig.Signature,
			map[string]interface{}{
				"encoding":                       "jsonParsed",
				"maxSupportedTransactionVersion": 0,
			},
		})
		if err != nil {
			continue
		}

		var parsed struct {
			BlockTime   *int64 `json:"blockTime"`
			Meta        *struct {
				Fee               int64 `json:"fee"`
				PreBalances       []int64 `json:"preBalances"`
				PostBalances      []int64 `json:"postBalances"`
				PreTokenBalances  []solTokenBalance `json:"preTokenBalances"`
				PostTokenBalances []solTokenBalance `json:"postTokenBalances"`
				Err               interface{} `json:"err"`
			} `json:"meta"`
			Transaction *struct {
				Message struct {
					AccountKeys []struct {
						Pubkey string `json:"pubkey"`
					} `json:"accountKeys"`
				} `json:"message"`
			} `json:"transaction"`
		}
		if json.Unmarshal(txResult, &parsed) != nil || parsed.Meta == nil || parsed.Meta.Err != nil {
			continue
		}

		ts := time.Time{}
		if parsed.BlockTime != nil {
			ts = time.Unix(*parsed.BlockTime, 0)
		}

		tx := db.WalletTransaction{
			WalletID:    walletID,
			TxHash:      sig.Signature,
			Chain:       config.ChainSolana,
			Timestamp:   ts,
			PriorityFee: float64(parsed.Meta.Fee),
		}

		// Find address index in account keys
		addrIdx := -1
		if parsed.Transaction != nil {
			for i, ak := range parsed.Transaction.Message.AccountKeys {
				if ak.Pubkey == address {
					addrIdx = i
					break
				}
			}
		}

		// Check SOL balance change
		if addrIdx >= 0 && addrIdx < len(parsed.Meta.PreBalances) && addrIdx < len(parsed.Meta.PostBalances) {
			solDiff := float64(parsed.Meta.PostBalances[addrIdx]-parsed.Meta.PreBalances[addrIdx]) / 1e9
			if solDiff > 0.001 {
				tx.TxType = "transfer_in"
				tx.TokenSymbol = "SOL"
				tx.AmountToken = solDiff
				tx.AmountUSD = solDiff * solPrice
			} else if solDiff < -0.001 {
				// Could be a swap (sent SOL, received tokens) or transfer out
				tx.AmountUSD = -solDiff * solPrice
			}
		}

		// Check token balance changes
		preTokens := mapTokenBalances(parsed.Meta.PreTokenBalances, address)
		postTokens := mapTokenBalances(parsed.Meta.PostTokenBalances, address)

		for mint, postAmt := range postTokens {
			preAmt := preTokens[mint]
			diff := postAmt - preAmt
			if diff > 0 {
				tx.TxType = "swap_buy"
				tx.TokenAddress = mint
				tx.AmountToken = diff
			} else if diff < 0 {
				tx.TxType = "swap_sell"
				tx.TokenAddress = mint
				tx.AmountToken = -diff
			}
		}
		// Tokens that disappeared entirely
		for mint, preAmt := range preTokens {
			if _, ok := postTokens[mint]; !ok && preAmt > 0 {
				tx.TxType = "swap_sell"
				tx.TokenAddress = mint
				tx.AmountToken = preAmt
			}
		}

		if tx.TxType != "" {
			if s.store.InsertTransaction(tx) == nil {
				count++
			}
		}
	}

	log.Info().Str("addr", abbrev(address)).Int("txs", count).Str("mode", "rpc").Msg("scanned solana")
	return count, nil
}

type solTokenBalance struct {
	AccountIndex  int `json:"accountIndex"`
	Mint          string `json:"mint"`
	Owner         string `json:"owner"`
	UITokenAmount struct {
		UIAmount *float64 `json:"uiAmount"`
	} `json:"uiTokenAmount"`
}

// mapTokenBalances extracts token amounts for a specific owner from pre/post balances.
func mapTokenBalances(balances []solTokenBalance, owner string) map[string]float64 {
	result := map[string]float64{}
	for _, b := range balances {
		if b.Owner == owner && b.UITokenAmount.UIAmount != nil {
			result[b.Mint] = *b.UITokenAmount.UIAmount
		}
	}
	return result
}
