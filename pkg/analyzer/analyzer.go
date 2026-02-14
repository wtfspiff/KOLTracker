package analyzer

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
)

type Analyzer struct {
	store *db.Store
	cfg   *config.Config
}

func New(cfg *config.Config, store *db.Store) *Analyzer {
	return &Analyzer{store: store, cfg: cfg}
}

type KOLFingerprint struct {
	KOLID           int64            `json:"kol_id"`
	TradeCount      int              `json:"trade_count"`
	BuySize         *SizePattern     `json:"buy_size,omitempty"`
	SellSize        *SizePattern     `json:"sell_size,omitempty"`
	PreferredDEX    map[string]int   `json:"preferred_dex"`
	PreferredRouter string           `json:"preferred_router"`
	PreferredBot    string           `json:"preferred_bot"`
	BotSignatures   map[string]int   `json:"bot_signatures"`
	GasProfile      *GasProfile      `json:"gas_profile,omitempty"`
	TimingProfile   *TimingProfile   `json:"timing_profile,omitempty"`
	SellBehavior    *SellBehavior    `json:"sell_behavior,omitempty"`
	TokenPrefs      *TokenPrefs      `json:"token_prefs,omitempty"`
	CEXProfile      *CEXProfile      `json:"cex_profile,omitempty"`
	ENSProfile      *ENSProfile      `json:"ens_profile,omitempty"`
	DepositPattern  *DepositPattern  `json:"deposit_pattern,omitempty"`
	ActivityHeatmap [24]int          `json:"activity_heatmap"`
	ChainPreference map[string]int   `json:"chain_preference"`
}

type SizePattern struct {
	Min, Max, Mean, Median, StdDev, P25, P75 float64
	CommonAmounts []CommonAmount `json:"common_amounts"`
	Buckets       map[string]int `json:"buckets"`
	SampleCount   int            `json:"sample_count"`
}
type CommonAmount struct{ Amount float64; Count int }
type GasProfile struct {
	AvgFee, MedianFee, MinFee, MaxFee, StdDevFee float64
	CommonFees []CommonAmount `json:"common_fees"`
	UsesJito   bool           `json:"uses_jito"`
	AvgJitoTip float64        `json:"avg_jito_tip"`
	SampleCount int           `json:"sample_count"`
}
type TimingProfile struct {
	AvgPreBuySec, AvgPostBuySec, MedianPreBuySec, MedianPostBuySec float64
	PreBuyCount, PostBuyCount int
	PreBuyPct float64
	AvgHoldMin, MedianHoldMin float64
	DayOfWeek [7]int `json:"day_of_week"`
}
type SellBehavior struct {
	AvgChunks float64; CommonSellPcts []float64; AvgProfitPct float64
}
type TokenPrefs struct {
	UniqueTokens int; RepeatBuyPct float64
}
type CEXProfile struct {
	UsedCEXes map[string]int `json:"used_cexes"`
	CommonDepositAmts []CommonAmount `json:"common_deposit_amts"`
}
type ENSProfile struct {
	HasENS bool; ENSNames []string; HasSNS bool; SNSNames []string
	NamingPattern string `json:"naming_pattern"`
}
type DepositPattern struct {
	CommonDeposits []CommonAmount `json:"common_deposits"`
	CommonWithdraws []CommonAmount `json:"common_withdraws"`
	RoundNumberPct float64 `json:"round_number_pct"`
	PreferredSize float64 `json:"preferred_size"`
}

func (a *Analyzer) BuildKOLFingerprint(kolID int64) (*KOLFingerprint, error) {
	wallets, err := a.store.GetWalletsForKOL(kolID)
	if err != nil { return nil, err }
	fp := &KOLFingerprint{KOLID: kolID, PreferredDEX: map[string]int{}, BotSignatures: map[string]int{}, ChainPreference: map[string]int{}}
	var allTrades []db.WalletTransaction
	for _, w := range wallets {
		if w.Confidence < 0.5 { continue }
		trades, _ := a.store.GetTransactionsForWallet(w.ID, 1000)
		allTrades = append(allTrades, trades...)
		fp.ChainPreference[string(w.Chain)] += len(trades)
	}
	if len(allTrades) == 0 { return fp, nil }
	fp.TradeCount = len(allTrades)
	var buyAmts, sellAmts, fees []float64
	tokenBuys := map[string]int{}
	tokenFirst := map[string]time.Time{}
	tokenLastSell := map[string]time.Time{}
	for _, t := range allTrades {
		switch t.TxType {
		case "swap_buy":
			if t.AmountUSD > 0 { buyAmts = append(buyAmts, t.AmountUSD) }
			tokenBuys[t.TokenAddress]++
			if f, ok := tokenFirst[t.TokenAddress]; !ok || t.Timestamp.Before(f) { tokenFirst[t.TokenAddress] = t.Timestamp }
		case "swap_sell":
			if t.AmountUSD > 0 { sellAmts = append(sellAmts, t.AmountUSD) }
			if l, ok := tokenLastSell[t.TokenAddress]; !ok || t.Timestamp.After(l) { tokenLastSell[t.TokenAddress] = t.Timestamp }
		}
		if t.Platform != "" { fp.PreferredDEX[t.Platform]++ }
		if t.PriorityFee > 0 { fees = append(fees, t.PriorityFee) }
		if !t.Timestamp.IsZero() { fp.ActivityHeatmap[t.Timestamp.UTC().Hour()]++ }
	}
	if len(buyAmts) > 0 { fp.BuySize = buildSizePattern(buyAmts); a.store.UpsertPattern(kolID, "buy_size_range", fp.BuySize, len(buyAmts)) }
	if len(sellAmts) > 0 { fp.SellSize = buildSizePattern(sellAmts); a.store.UpsertPattern(kolID, "sell_pattern", fp.SellSize, len(sellAmts)) }
	if len(fp.PreferredDEX) > 0 { fp.PreferredRouter = topKey(fp.PreferredDEX); a.store.UpsertPattern(kolID, "preferred_dex", fp.PreferredDEX, fp.TradeCount) }
	if len(fees) > 0 { fp.GasProfile = buildGasProfile(fees); a.store.UpsertPattern(kolID, "gas_priority", fp.GasProfile, len(fees)) }
	fp.TimingProfile = a.buildTimingProfile(kolID, allTrades, tokenFirst, tokenLastSell)
	if fp.TimingProfile != nil { a.store.UpsertPattern(kolID, "timing_pattern", fp.TimingProfile, fp.TimingProfile.PreBuyCount+fp.TimingProfile.PostBuyCount) }
	repeatBuys := 0; for _, c := range tokenBuys { if c > 1 { repeatBuys++ } }
	fp.TokenPrefs = &TokenPrefs{UniqueTokens: len(tokenBuys), RepeatBuyPct: safePct(repeatBuys, len(tokenBuys))}
	a.store.UpsertPattern(kolID, "full_fingerprint", fp, fp.TradeCount)
	log.Info().Int64("kol", kolID).Int("trades", fp.TradeCount).Msg("📊 built KOL fingerprint")
	return fp, nil
}

// Alias for backward compat
func (a *Analyzer) BuildKOLProfile(kolID int64) (*KOLFingerprint, error) {
	return a.BuildKOLFingerprint(kolID)
}

func buildSizePattern(amounts []float64) *SizePattern {
	sort.Float64s(amounts); n := len(amounts)
	p := &SizePattern{Min: amounts[0], Max: amounts[n-1], Mean: avg(amounts), Median: amounts[n/2], P25: amounts[n/4], P75: amounts[3*n/4], SampleCount: n, Buckets: map[string]int{}}
	sumSq := 0.0; for _, a := range amounts { sumSq += (a - p.Mean) * (a - p.Mean) }; p.StdDev = math.Sqrt(sumSq / float64(n))
	for _, a := range amounts {
		switch { case a < 50: p.Buckets["$0-50"]++; case a < 100: p.Buckets["$50-100"]++; case a < 250: p.Buckets["$100-250"]++; case a < 500: p.Buckets["$250-500"]++; case a < 1000: p.Buckets["$500-1K"]++; case a < 5000: p.Buckets["$1K-5K"]++; default: p.Buckets["$5K+"]++ }
	}
	rounded := map[float64]int{}
	for _, a := range amounts { key := math.Round(a/10) * 10; if a < 100 { key = math.Round(a) }; rounded[key]++ }
	for amt, count := range rounded { if count >= 2 { p.CommonAmounts = append(p.CommonAmounts, CommonAmount{amt, count}) } }
	sort.Slice(p.CommonAmounts, func(i, j int) bool { return p.CommonAmounts[i].Count > p.CommonAmounts[j].Count })
	if len(p.CommonAmounts) > 15 { p.CommonAmounts = p.CommonAmounts[:15] }
	return p
}

func buildGasProfile(fees []float64) *GasProfile {
	sort.Float64s(fees); n := len(fees)
	g := &GasProfile{AvgFee: avg(fees), MedianFee: fees[n/2], MinFee: fees[0], MaxFee: fees[n-1], SampleCount: n}
	sumSq := 0.0; for _, f := range fees { sumSq += (f - g.AvgFee) * (f - g.AvgFee) }; g.StdDevFee = math.Sqrt(sumSq / float64(n))
	rounded := map[float64]int{}; for _, f := range fees { rounded[math.Round(f/1000)*1000]++ }
	for amt, c := range rounded { if c >= 2 { g.CommonFees = append(g.CommonFees, CommonAmount{amt, c}) } }
	jitoCount := 0; var jitoTips []float64; for _, f := range fees { if f > 10000 { jitoCount++; jitoTips = append(jitoTips, f) } }
	if jitoCount > n/3 { g.UsesJito = true; g.AvgJitoTip = avg(jitoTips) }
	return g
}

func (a *Analyzer) buildTimingProfile(kolID int64, trades []db.WalletTransaction, tokenFirst, tokenLastSell map[string]time.Time) *TimingProfile {
	mentions, _ := a.store.GetRecentTokenMentions(720)
	if len(mentions) == 0 { return nil }
	var preDiffs, postDiffs []float64
	dw := [7]int{}
	for _, t := range trades { if !t.Timestamp.IsZero() { dw[t.Timestamp.Weekday()]++ }
		if t.TokenAddress == "" || t.Timestamp.IsZero() { continue }
		for _, m := range mentions {
			if m.KOLID == kolID && m.TokenAddress == t.TokenAddress && !m.MentionedAt.IsZero() {
				d := t.Timestamp.Sub(m.MentionedAt).Seconds()
				if math.Abs(d) < 86400 { if d < 0 { preDiffs = append(preDiffs, d) } else { postDiffs = append(postDiffs, d) } }
			}
		}
	}
	total := len(preDiffs) + len(postDiffs); if total == 0 { return nil }
	tp := &TimingProfile{PreBuyCount: len(preDiffs), PostBuyCount: len(postDiffs), PreBuyPct: safePct(len(preDiffs), total), DayOfWeek: dw}
	if len(preDiffs) > 0 { sort.Float64s(preDiffs); tp.AvgPreBuySec = avg(preDiffs); tp.MedianPreBuySec = preDiffs[len(preDiffs)/2] }
	if len(postDiffs) > 0 { sort.Float64s(postDiffs); tp.AvgPostBuySec = avg(postDiffs); tp.MedianPostBuySec = postDiffs[len(postDiffs)/2] }
	var holdTimes []float64
	for tok, fb := range tokenFirst { if ls, ok := tokenLastSell[tok]; ok { h := ls.Sub(fb).Minutes(); if h > 0 && h < 43200 { holdTimes = append(holdTimes, h) } } }
	if len(holdTimes) > 0 { sort.Float64s(holdTimes); tp.AvgHoldMin = avg(holdTimes); tp.MedianHoldMin = holdTimes[len(holdTimes)/2] }
	return tp
}

func (a *Analyzer) ScoreWashCandidate(kolID int64, address string, chain config.Chain) (*db.WashScore, error) {
	ws := &db.WashScore{Address: address, Chain: chain, Signals: map[string]interface{}{}}
	patterns, _ := a.store.GetPatternsForKOL(kolID)
	pm := map[string]string{}; for _, p := range patterns { pm[p.PatternType] = p.PatternData }
	score := 0.0
	// 1-Token overlap
	if s, sig := a.scoreTokenOverlap(kolID, address); s > 0 { score += s; ws.Signals["token_overlap"] = sig }
	// 2-Timing
	if s, sig := a.scoreTimingCorr(kolID, address); s > 0 { score += s; ws.Signals["timing"] = sig }
	// 3-Amount
	if raw, ok := pm["buy_size_range"]; ok { var bp SizePattern; json.Unmarshal([]byte(raw), &bp)
		if s, sig := a.scoreAmount(address, &bp); s > 0 { score += s; ws.Signals["amount"] = sig } }
	// 4-DEX
	if raw, ok := pm["preferred_dex"]; ok { var dp map[string]int; json.Unmarshal([]byte(raw), &dp)
		if s, sig := a.scoreDEX(address, dp); s > 0 { score += s; ws.Signals["dex"] = sig } }
	// 5-Gas
	if raw, ok := pm["gas_priority"]; ok { var gp GasProfile; json.Unmarshal([]byte(raw), &gp)
		if s, sig := a.scoreGas(address, &gp); s > 0 { score += s; ws.Signals["gas"] = sig } }
	// 6-Funding source
	if s, sig := a.scoreFunding(address, chain); s > 0 { score += s; ws.Signals["funding"] = sig }
	// 7-Wallet age
	if s, sig := a.scoreAge(address, chain); s > 0 { score += s; ws.Signals["age"] = sig }
	ws.TotalScore = math.Min(score, 1.0)
	sigs := map[string]bool{}
	if _, ok := ws.Signals["token_overlap"]; ok { sigs["bought_same_token"] = true }
	if _, ok := ws.Signals["timing"]; ok { sigs["timing_match"] = true }
	if _, ok := ws.Signals["amount"]; ok { sigs["amount_pattern_match"] = true }
	if _, ok := ws.Signals["gas"]; ok { sigs["bot_signature_match"] = true }
	a.store.UpdateWashScore(address, chain, ws.TotalScore, sigs)
	if ws.TotalScore >= a.cfg.WashWalletMinScore {
		sev := "info"; if ws.TotalScore >= 0.7 { sev = "critical" } else if ws.TotalScore >= 0.5 { sev = "warning" }
		sj, _ := json.Marshal(ws.Signals)
		a.store.InsertAlert(kolID, "wash_wallet", sev, fmt.Sprintf("Wash wallet: %s (%.0f%%)", abbrev(address), ws.TotalScore*100), string(sj), address, "")
	}
	return ws, nil
}

func (a *Analyzer) scoreTokenOverlap(kolID int64, addr string) (float64, map[string]interface{}) {
	mentions, _ := a.store.GetRecentTokenMentions(168); kt := map[string]bool{}
	for _, m := range mentions { if m.KOLID == kolID && m.TokenAddress != "" { kt[m.TokenAddress] = true } }
	buys, _ := a.store.GetBuyTransactionsForAddress(addr); o := 0
	for _, b := range buys { if kt[b.TokenAddress] { o++ } }
	if o == 0 { return 0, nil }
	return math.Min(float64(o)*0.1, 0.3), map[string]interface{}{"overlap": o}
}

func (a *Analyzer) scoreTimingCorr(kolID int64, addr string) (float64, map[string]interface{}) {
	mentions, _ := a.store.GetRecentTokenMentions(168); mt := map[string][]time.Time{}
	for _, m := range mentions { if m.KOLID == kolID && m.TokenAddress != "" { mt[m.TokenAddress] = append(mt[m.TokenAddress], m.MentionedAt) } }
	buys, _ := a.store.GetBuyTransactionsForAddress(addr); corr, tot := 0, 0
	for _, b := range buys { if times, ok := mt[b.TokenAddress]; ok { tot++; for _, t := range times { if math.Abs(b.Timestamp.Sub(t).Seconds()) < 7200 { corr++; break } } } }
	if tot == 0 || safePct(corr, tot) < 50 { return 0, nil }
	return 0.2, map[string]interface{}{"pct": safePct(corr, tot)}
}

func (a *Analyzer) scoreAmount(addr string, kp *SizePattern) (float64, map[string]interface{}) {
	buys, _ := a.store.GetBuyTransactionsForAddress(addr); var amts []float64
	for _, b := range buys { if b.AmountUSD > 0 { amts = append(amts, b.AmountUSD) } }
	if len(amts) == 0 || kp.Mean == 0 { return 0, nil }
	d := math.Abs(avg(amts)-kp.Mean) / kp.Mean * 100
	cm := 0; for _, a := range amts { for _, ka := range kp.CommonAmounts { if ka.Amount > 0 && math.Abs(a-ka.Amount)/ka.Amount < 0.15 { cm++; break } } }
	if d > 30 && cm < 2 { return 0, nil }
	return 0.15, map[string]interface{}{"diff_pct": d, "common": cm}
}

func (a *Analyzer) scoreDEX(addr string, kd map[string]int) (float64, map[string]interface{}) {
	buys, _ := a.store.GetBuyTransactionsForAddress(addr); cd := map[string]int{}
	for _, b := range buys { if b.Platform != "" { cd[b.Platform]++ } }
	if len(cd) == 0 || topKey(kd) != topKey(cd) { return 0, nil }
	return 0.1, map[string]interface{}{"dex": topKey(kd)}
}

func (a *Analyzer) scoreGas(addr string, kp *GasProfile) (float64, map[string]interface{}) {
	buys, _ := a.store.GetBuyTransactionsForAddress(addr); var fees []float64
	for _, b := range buys { if b.PriorityFee > 0 { fees = append(fees, b.PriorityFee) } }
	if len(fees) == 0 || kp.AvgFee == 0 { return 0, nil }
	d := math.Abs(avg(fees)-kp.AvgFee) / kp.AvgFee * 100
	if d > 20 { return 0, nil }
	return 0.15, map[string]interface{}{"diff_pct": d}
}

func (a *Analyzer) scoreFunding(addr string, chain config.Chain) (float64, map[string]interface{}) {
	cs, _ := a.store.GetWashCandidates(0.0)
	for _, c := range cs { if c.Address == addr && c.Chain == chain {
		switch c.FundingSourceType {
		case "fixedfloat": return 0.30, map[string]interface{}{"type": "fixedfloat", "amount": c.FundingAmount}
		case "bridge": return 0.20, map[string]interface{}{"type": "bridge"}
		case "mixer": return 0.35, map[string]interface{}{"type": "mixer"}
		case "swap_service": return 0.25, map[string]interface{}{"type": "swap_service"}
		}
	}}; return 0, nil
}

func (a *Analyzer) scoreAge(addr string, chain config.Chain) (float64, map[string]interface{}) {
	cs, _ := a.store.GetWashCandidates(0.0)
	for _, c := range cs { if c.Address == addr && c.Chain == chain && !c.FirstSeen.IsZero() {
		age := time.Since(c.FirstSeen)
		if age < 24*time.Hour { return 0.2, map[string]interface{}{"hours": age.Hours()} }
		if age < 7*24*time.Hour { return 0.1, map[string]interface{}{"days": age.Hours() / 24} }
	}}; return 0, nil
}

func (a *Analyzer) MatchFundingAmounts(kolID int64, tolerancePct float64, windowHours int) ([]db.FundingFlowMatch, error) {
	wallets, _ := a.store.GetWalletsForKOL(kolID); var matches []db.FundingFlowMatch
	for _, w := range wallets { txs, _ := a.store.GetTransactionsForWallet(w.ID, 200); cands, _ := a.store.GetWashCandidates(0.0)
		for _, out := range txs { if out.TxType != "transfer_out" || out.AmountToken == 0 { continue }
			for _, c := range cands { if c.FundingAmount == 0 { continue }
				d := math.Abs(out.AmountToken-c.FundingAmount) / out.AmountToken * 100
				if d > tolerancePct { continue }
				var td int64; if !out.Timestamp.IsZero() && !c.FirstSeen.IsZero() { td = int64(math.Abs(c.FirstSeen.Sub(out.Timestamp).Seconds())); if td > int64(windowHours)*3600 { continue } }
				conf := 1.0 - (d/tolerancePct)*0.5
				fm := db.FundingFlowMatch{SourceTx: out.TxHash, SourceChain: out.Chain, SourceAmount: out.AmountToken, SourceToken: out.TokenSymbol, DestAddress: c.Address, DestChain: c.Chain, DestAmount: c.FundingAmount, DestToken: c.FundingToken, Service: c.FundingSourceType, AmountDiffPct: d, TimeDiffSeconds: td, MatchConfidence: conf}
				a.store.InsertFundingMatch(fm); matches = append(matches, fm)
	}}}; return matches, nil
}

func avg(v []float64) float64 { if len(v)==0{return 0}; s:=0.0; for _,x:=range v{s+=x}; return s/float64(len(v)) }
func topKey(m map[string]int) string { t,mx:="",0; for k,v:=range m{if v>mx{t,mx=k,v}}; return t }
func safePct(n,t int) float64 { if t==0{return 0}; return float64(n)/float64(t)*100 }
func abbrev(a string) string { if len(a)>12{return a[:6]+"..."+a[len(a)-4:]}; return a }

