package extractor

import (
	"regexp"
	"strings"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
)

var (
	// Address patterns
	solanaAddrRe = regexp.MustCompile(`\b([1-9A-HJ-NP-Za-km-z]{32,44})\b`)
	evmAddrRe    = regexp.MustCompile(`\b(0x[a-fA-F0-9]{40})\b`)
	tickerRe     = regexp.MustCompile(`\$([A-Za-z][A-Za-z0-9]{1,10})\b`)

	// DEX/tool link patterns
	dexscreenerRe = regexp.MustCompile(`https?://(?:www\.)?dexscreener\.com/[^\s\)\]]+`)
	birdeyeRe     = regexp.MustCompile(`https?://(?:www\.)?birdeye\.so/[^\s\)\]]+`)
	pumpfunRe     = regexp.MustCompile(`https?://(?:www\.)?pump\.fun/[^\s\)\]]+`)
	photonRe      = regexp.MustCompile(`https?://(?:www\.)?photon-sol\.tinyastro\.io/[^\s\)\]]+`)
	gmgnRe        = regexp.MustCompile(`https?://(?:www\.)?gmgn\.ai/[^\s\)\]]+`)
	bullxRe       = regexp.MustCompile(`https?://(?:www\.)?bullx\.io/[^\s\)\]]+`)
	axiomRe       = regexp.MustCompile(`https?://(?:www\.)?axiom\.trade/[^\s\)\]]+`)
	genericURLRe  = regexp.MustCompile(`https?://[^\s\)\]]+`)

	// Bot detection patterns
	botPatterns = map[string]*regexp.Regexp{
		"bonkbot":     regexp.MustCompile(`(?i)bonkbot|@bonkbot`),
		"trojan":      regexp.MustCompile(`(?i)trojan|@trojanonsol`),
		"maestro":     regexp.MustCompile(`(?i)maestro.*bot|@maestro`),
		"banana_gun":  regexp.MustCompile(`(?i)banana\s*gun|@bananagun`),
		"sol_trading":  regexp.MustCompile(`(?i)@SolTradingBot`),
		"photon":      regexp.MustCompile(`(?i)\bphoton\b`),
		"bullx":       regexp.MustCompile(`(?i)\bbullx\b`),
		"pepeboost":   regexp.MustCompile(`(?i)pepeboost`),
		"bloom":       regexp.MustCompile(`(?i)@BloomSolana|bloom\s*bot`),
		"axiom":       regexp.MustCompile(`(?i)\baxiom\b`),
		"ray_bot":     regexp.MustCompile(`(?i)@ray_silver_bot`),
	}

	buySignalRe  = regexp.MustCompile(`(?i)\b(buy|bought|buying|ape[ds]?|entry|long|accumulating|loaded|scooped)\b`)
	sellSignalRe = regexp.MustCompile(`(?i)\b(sell|sold|selling|exit|short|dump|take\s*profit|tp|trimmed|closed)\b`)

	// False positive address filters
	falsePositives = map[string]bool{
		"SOL": true, "USDC": true, "USDT": true, "BONK": true, "WIF": true,
		"JUP": true, "RAY": true, "ORCA": true, "Twitter": true, "Telegram": true,
		"Discord": true, "https": true, "http": true, "pump": true, "solana": true,
		"ethereum": true, "bitcoin": true, "lamports": true,
	}

	// Noise tickers to skip
	noiseTickers = map[string]bool{
		"USD": true, "EUR": true, "GBP": true, "BTC": true, "ETH": true,
		"NFT": true, "DM": true, "RT": true, "DYOR": true, "NFA": true,
		"IMO": true, "TBH": true, "ATH": true, "ATL": true, "APY": true,
		"TVL": true, "CEO": true, "DEX": true, "CEX": true, "DCA": true,
		"FUD": true, "HODL": true, "FOMO": true, "WAGMI": true,
	}
)

// Extract parses social media text and extracts all wallet addresses, token CAs,
// ticker symbols, and relevant links.
func Extract(text string) *db.ExtractionResult {
	r := &db.ExtractionResult{
		BotSignals: make(map[string]bool),
	}

	// 1. Extract all typed links first
	r.DexScreenerLinks = dexscreenerRe.FindAllString(text, -1)
	r.BirdeyeLinks = birdeyeRe.FindAllString(text, -1)
	r.PumpFunLinks = pumpfunRe.FindAllString(text, -1)
	r.PhotonLinks = photonRe.FindAllString(text, -1)
	r.GmgnLinks = gmgnRe.FindAllString(text, -1)
	r.BullxLinks = bullxRe.FindAllString(text, -1)

	// Extract token CAs from links
	allLinks := concat(r.DexScreenerLinks, r.BirdeyeLinks, r.PumpFunLinks,
		r.PhotonLinks, r.GmgnLinks, r.BullxLinks)
	axiomLinks := axiomRe.FindAllString(text, -1)
	allLinks = append(allLinks, axiomLinks...)

	for _, link := range allLinks {
		if ca := extractCAFromLink(link); ca != "" {
			r.TokenCAsFromLinks = appendUnique(r.TokenCAsFromLinks, ca)
		}
	}

	// Remove links from text before address extraction to avoid false positives
	cleanText := text
	for _, link := range allLinks {
		cleanText = strings.Replace(cleanText, link, " ", 1)
	}

	// Also grab remaining generic URLs
	otherURLs := genericURLRe.FindAllString(cleanText, -1)
	for _, u := range otherURLs {
		cleanText = strings.Replace(cleanText, u, " ", 1)
		// Try extracting CAs from generic links too
		if ca := extractCAFromLink(u); ca != "" {
			r.TokenCAsFromLinks = appendUnique(r.TokenCAsFromLinks, ca)
		}
		r.OtherLinks = append(r.OtherLinks, u)
	}

	// 2. Extract EVM addresses
	evmMatches := evmAddrRe.FindAllString(cleanText, -1)
	for _, addr := range evmMatches {
		r.EVMAddresses = appendUnique(r.EVMAddresses, addr)
	}

	// 3. Extract Solana addresses (filter aggressively)
	solMatches := solanaAddrRe.FindAllString(cleanText, -1)
	for _, addr := range solMatches {
		if isValidSolanaAddress(addr) {
			r.SolanaAddresses = appendUnique(r.SolanaAddresses, addr)
		}
	}

	// 4. Extract $TICKER mentions
	tickerMatches := tickerRe.FindAllStringSubmatch(text, -1)
	for _, m := range tickerMatches {
		if len(m) > 1 {
			ticker := strings.ToUpper(m[1])
			if !noiseTickers[ticker] {
				r.TokenSymbols = appendUnique(r.TokenSymbols, ticker)
			}
		}
	}

	// 5. All standalone addresses are potential CAs too
	r.ContractAddrs = append(r.SolanaAddresses, r.EVMAddresses...)

	// 6. Detect trading bot mentions
	for name, re := range botPatterns {
		if re.MatchString(text) {
			r.BotSignals[name] = true
		}
	}

	// 7. Detect buy/sell signals
	r.BuySignal = buySignalRe.MatchString(text)
	r.SellSignal = sellSignalRe.MatchString(text)

	return r
}

// ClassifyAddress determines chain for an address
func ClassifyAddress(addr string) config.Chain {
	if strings.HasPrefix(addr, "0x") && len(addr) == 42 {
		return config.ChainEthereum // default EVM, caller can refine
	}
	return config.ChainSolana
}

func isValidSolanaAddress(addr string) bool {
	if len(addr) < 32 || len(addr) > 44 {
		return false
	}
	if falsePositives[addr] {
		return false
	}
	hasUpper, hasLower, hasDigit := false, false, false
	for _, c := range addr {
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		} else if c >= 'a' && c <= 'z' {
			hasLower = true
		} else if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}
	return hasUpper && hasLower && hasDigit
}

func extractCAFromLink(url string) string {
	url = strings.TrimRight(url, "/")
	// Remove query params and fragments
	if idx := strings.Index(url, "?"); idx > 0 {
		// But check query params for token addresses too
		query := url[idx+1:]
		url = url[:idx]
		for _, param := range strings.Split(query, "&") {
			parts := strings.SplitN(param, "=", 2)
			if len(parts) == 2 {
				val := parts[1]
				if isValidSolanaAddress(val) || evmAddrRe.MatchString(val) {
					return val
				}
			}
		}
	}

	parts := strings.Split(url, "/")
	// Walk backwards to find the first thing that looks like an address
	for i := len(parts) - 1; i >= 0; i-- {
		segment := strings.TrimSpace(parts[i])
		if segment == "" {
			continue
		}
		if evmAddrRe.MatchString(segment) {
			return segment
		}
		if isValidSolanaAddress(segment) {
			return segment
		}
	}
	return ""
}

func appendUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}

func concat(slices ...[]string) []string {
	var result []string
	for _, s := range slices {
		result = append(result, s...)
	}
	return result
}
