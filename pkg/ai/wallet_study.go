package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/kol-tracker/pkg/config"
	"github.com/kol-tracker/pkg/db"
)

// ============================================================================
// AI-POWERED WALLET STUDY ENGINE
// ============================================================================
// This is the brain of the wallet study system. When a known wallet is added
// for a KOL, the basic scanner collects raw on-chain data, then this AI engine
// analyzes that data to:
//
// 1. BUILD BEHAVIORAL FINGERPRINT: LLM creates a comprehensive profile of
//    trading behavior, timing patterns, fee signatures, DEX preferences.
//
// 2. DEEP RELATIONSHIP INFERENCE: Given the KOL's known wallets + all
//    discovered candidates, the AI determines which are truly linked based
//    on behavioral similarity, NOT just on-chain transfers.
//
// 3. PREDICTIVE WALLET DISCOVERY: Based on the KOL's behavior patterns,
//    the AI predicts what OTHER wallets to look for (e.g., "this KOL likely
//    has a Solana sniping wallet that buys within 3 seconds of LP creation,
//    look for wallets that bought tokens X,Y,Z at exactly those times").
//
// 4. FUNDING FLOW INTELLIGENCE: Traces complex multi-hop funding paths
//    and uses the LLM to determine if the path looks intentionally obfuscated.
//
// 5. CROSS-KOL NETWORK MAPPING: Detects if the same wash wallet infrastructure
//    is shared between multiple KOLs (syndicate detection).
//
// 6. CONTINUOUS CONFIDENCE REFINEMENT: As more data arrives, the AI
//    re-evaluates all wallet attributions and adjusts confidence scores.

// AIStudyResult is the comprehensive output from an AI-powered wallet study.
type AIStudyResult struct {
	WalletAddress   string              `json:"wallet_address"`
	Chain           config.Chain        `json:"chain"`
	BehaviorProfile *BehaviorProfile    `json:"behavior_profile"`
	LinkedWallets   []AILinkedWallet    `json:"linked_wallets"`
	PredictedWallets []PredictedWallet  `json:"predicted_wallets"`
	FundingAnalysis *FundingAnalysis    `json:"funding_analysis"`
	NetworkLinks    []NetworkLink       `json:"network_links"`
	RiskAssessment  *RiskAssessment     `json:"risk_assessment"`
	TokensToWatch   []string            `json:"tokens_to_watch"`
	Recommendations []string            `json:"recommendations"`
	LLMCallsUsed   int                  `json:"llm_calls_used"`
	TotalTokensUsed int                 `json:"total_tokens_used"`
}

// BehaviorProfile is an AI-generated comprehensive trading fingerprint.
type BehaviorProfile struct {
	TradingStyle       string   `json:"trading_style"`        // "sniper","swing","degen","conservative","bot_driven"
	RiskTolerance      string   `json:"risk_tolerance"`       // "extreme","high","medium","low"
	PreferredChains    []string `json:"preferred_chains"`
	PreferredDEXs      []string `json:"preferred_dexs"`
	AvgPositionSize    string   `json:"avg_position_size"`    // "$500-$2000"
	TypicalHoldTime    string   `json:"typical_hold_time"`    // "minutes","hours","days","weeks"
	ActiveHours        string   `json:"active_hours"`         // "US evening 8pm-2am UTC"
	BotUsage           string   `json:"bot_usage"`            // "heavy:trojan,bonkbot","light","none"
	GasStrategy        string   `json:"gas_strategy"`         // "high_priority","medium","frugal"
	EntryPattern       string   `json:"entry_pattern"`        // "dca","single_large_buy","multi_wallet_spread"
	ExitPattern        string   `json:"exit_pattern"`         // "quick_flip","staged_sell","hold_to_zero"
	UniqueSignatures   []string `json:"unique_signatures"`    // bot-specific fees, specific slippage settings, etc.
	SimilarToKOLMain   float64  `json:"similar_to_kol_main"`  // 0-1 similarity score to KOL's main wallet
	Summary            string   `json:"summary"`
}

// AILinkedWallet is a wallet the AI believes is controlled by the same entity.
type AILinkedWallet struct {
	Address       string   `json:"address"`
	Chain         string   `json:"chain"`
	Relationship  string   `json:"relationship"`   // "same_owner","wash","sniping","funding_hub","linked_entity"
	Confidence    float64  `json:"confidence"`
	Evidence      []string `json:"evidence"`
	BehaviorMatch float64  `json:"behavior_match"` // 0-1 how similar the behavior is
}

// PredictedWallet describes a wallet the AI thinks should EXIST but hasn't been found yet.
type PredictedWallet struct {
	Description   string   `json:"description"`    // "Solana sniping wallet that buys within 5s of LP"
	Chain         string   `json:"chain"`
	SearchCriteria string  `json:"search_criteria"` // how to find it
	Confidence    float64  `json:"confidence"`
	Priority      string   `json:"priority"`        // "high","medium","low"
}

// FundingAnalysis is the AI's assessment of how a wallet was funded.
type FundingAnalysis struct {
	PrimaryFunding     string   `json:"primary_funding"`      // "fixedfloat","bridge","cex","direct_transfer"
	FundingChain       []string `json:"funding_chain"`        // multi-hop path description
	IntentionallyObfuscated bool `json:"intentionally_obfuscated"`
	ObfuscationMethods []string `json:"obfuscation_methods"`  // "fixedfloat","chain_hop","mixer","time_delay"
	EstimatedTotalFunded float64 `json:"estimated_total_funded"`
	Reasoning          string   `json:"reasoning"`
}

// NetworkLink connects this wallet to other KOLs or known entities.
type NetworkLink struct {
	Entity      string  `json:"entity"`       // KOL name or "unknown_syndicate"
	Relationship string `json:"relationship"` // "shared_infrastructure","same_funding_source","co_trading"
	Evidence    string  `json:"evidence"`
	Confidence  float64 `json:"confidence"`
}

// RiskAssessment is the AI's overall evaluation of wash trading risk.
type RiskAssessment struct {
	OverallRisk       string  `json:"overall_risk"`        // "critical","high","medium","low","clean"
	WashTradingProb   float64 `json:"wash_trading_prob"`   // 0-1
	InsiderTradingProb float64 `json:"insider_trading_prob"` // 0-1
	PreBuyingProb     float64 `json:"pre_buying_prob"`     // 0-1
	PumpAndDumpProb   float64 `json:"pump_and_dump_prob"`  // 0-1
	Evidence          []string `json:"evidence"`
	Reasoning         string  `json:"reasoning"`
}

// AIWalletStudy performs a comprehensive AI-powered analysis of a wallet.
// This is THE core intelligence function of the tracker.
//
// It makes multiple LLM calls in sequence:
// 1. Behavior fingerprinting (1 call)
// 2. Relationship analysis vs known KOL wallets (1 call per known wallet, batched)
// 3. Funding flow intelligence (1 call)
// 4. Predictive wallet discovery (1 call)
// 5. Risk assessment synthesis (1 call)
//
// Typical cost per full study: 5-8 LLM calls ≈ $0.03-$0.08 with Sonnet
func (e *Engine) AIWalletStudy(ctx context.Context, kolID int64, address string, chain config.Chain) (*AIStudyResult, error) {
	if !e.IsEnabled() {
		return nil, fmt.Errorf("AI engine not enabled")
	}

	result := &AIStudyResult{
		WalletAddress: address,
		Chain:         chain,
	}

	kol, _ := e.store.GetKOLByID(kolID)
	if kol == nil {
		return nil, fmt.Errorf("KOL not found")
	}

	// Get the wallet record
	wallet, err := e.store.GetWalletByAddress(address, chain)
	if err != nil || wallet == nil {
		return nil, fmt.Errorf("wallet not in database yet")
	}

	log.Info().Str("wallet", abbrev(address)).Str("kol", kol.Name).
		Msg("🧠 starting AI wallet study")

	// Build profile for the target wallet
	targetProfile := e.BuildWalletProfile(wallet.ID, address, chain)

	// Build profiles for ALL known KOL wallets (for comparison)
	knownWallets, _ := e.store.GetWalletsForKOL(kolID)
	var knownProfiles []WalletProfile
	var knownSummary strings.Builder
	for _, kw := range knownWallets {
		if kw.Address == address {
			continue // skip self
		}
		p := e.BuildWalletProfile(kw.ID, kw.Address, kw.Chain)
		knownProfiles = append(knownProfiles, p)
		knownSummary.WriteString(formatWalletSummary(kw, p))
	}

	// Get recent social posts for context
	posts, _ := e.store.GetPostsForKOL(kolID, 30)
	var postContext strings.Builder
	for _, p := range posts {
		postContext.WriteString(fmt.Sprintf("[%s] %s\n", p.PostedAt.Format("Jan 2 15:04"), truncate(p.Content, 150)))
	}

	// Get wash candidates for cross-reference
	washCandidates, _ := e.store.GetWashCandidatesForKOL(kolID)
	var washContext strings.Builder
	for _, c := range washCandidates {
		washContext.WriteString(fmt.Sprintf("  %s (%s) score:%.0f%% funding:%s\n",
			abbrev(c.Address), c.Chain, c.ConfidenceScore*100, c.FundingSourceType))
	}

	// ═══════════════════════════════════════════════════
	// CALL 1: BEHAVIORAL FINGERPRINTING
	// ═══════════════════════════════════════════════════
	behaviorPrompt := fmt.Sprintf(`You are an elite blockchain forensic analyst. Create a comprehensive behavioral fingerprint for this wallet.

KOL: %s (@%s)

TARGET WALLET: %s (%s)
Transactions: %d
Buy amounts: min=$%.2f, max=$%.2f, avg=$%.2f
Top DEX: %s
Avg priority fee: %.2f
Active hours: %s
Top tokens: %s
Funding: %s

KOL'S OTHER KNOWN WALLETS:
%s

Return JSON:
{
  "trading_style": "sniper|swing|degen|conservative|bot_driven|mixed",
  "risk_tolerance": "extreme|high|medium|low",
  "preferred_chains": ["chain1"],
  "preferred_dexs": ["dex1"],
  "avg_position_size": "$X-$Y range",
  "typical_hold_time": "seconds|minutes|hours|days|weeks",
  "active_hours": "description of when active in UTC",
  "bot_usage": "heavy:bot_names|light|none",
  "gas_strategy": "high_priority|medium|frugal|fixed_bot_fee",
  "entry_pattern": "dca|single_large_buy|multi_wallet_spread|snipe",
  "exit_pattern": "quick_flip|staged_sell|hold_to_zero|dump_all",
  "unique_signatures": ["specific fee amounts","specific slippage patterns","specific DEX router usage"],
  "similar_to_kol_main": 0.0-1.0,
  "summary": "2-3 sentence behavioral summary"
}
Return ONLY valid JSON.`,
		kol.Name, kol.TwitterHandle, abbrev(address), chain,
		targetProfile.TxCount, targetProfile.MinBuy, targetProfile.MaxBuy, targetProfile.AvgBuy,
		targetProfile.TopDEX, targetProfile.AvgFee, targetProfile.ActiveHours, targetProfile.TopTokens,
		targetProfile.FundingSource, knownSummary.String())

	resp, err := e.callLLM(ctx, behaviorPrompt, "json")
	if err != nil {
		log.Warn().Err(err).Msg("AI behavior analysis failed")
	} else {
		result.LLMCallsUsed++
		result.TotalTokensUsed += estimateTokens(behaviorPrompt, resp)
		var bp BehaviorProfile
		json.Unmarshal(extractJSON(resp), &bp)
		result.BehaviorProfile = &bp
		log.Info().Str("style", bp.TradingStyle).Float64("similarity", bp.SimilarToKOLMain).
			Msg("🧠 behavior fingerprint complete")
	}

	// ═══════════════════════════════════════════════════
	// CALL 2: RELATIONSHIP ANALYSIS (batched)
	// ═══════════════════════════════════════════════════
	if len(knownProfiles) > 0 {
		var comparisonData strings.Builder
		comparisonData.WriteString(fmt.Sprintf("TARGET: %s (%s)\n  Txs: %d, AvgBuy: $%.0f, DEX: %s, Fee: %.0f, Hours: %s, Tokens: %s\n\n",
			abbrev(address), chain, targetProfile.TxCount, targetProfile.AvgBuy,
			targetProfile.TopDEX, targetProfile.AvgFee, targetProfile.ActiveHours, targetProfile.TopTokens))

		for i, kp := range knownProfiles {
			kw := knownWallets[i]
			if kw.Address == address {
				continue
			}
			comparisonData.WriteString(fmt.Sprintf("KNOWN WALLET #%d: %s (%s, label: %s, conf: %.0f%%)\n  Txs: %d, AvgBuy: $%.0f, DEX: %s, Fee: %.0f, Hours: %s, Tokens: %s\n  Direct transfers to target: %v\n  Shared tokens: %d\n  Near-simultaneous trades: %d\n  Amount match: %v\n\n",
				i+1, abbrev(kp.Address), kp.Chain, kw.Label, kw.Confidence*100,
				kp.TxCount, kp.AvgBuy, kp.TopDEX, kp.AvgFee, kp.ActiveHours, kp.TopTokens,
				kp.DirectTransfersTo(address), kp.SharedTokenCount(targetProfile),
				kp.NearSimultaneousTrades(targetProfile), kp.AmountMatchesWith(targetProfile)))
		}

		relationPrompt := fmt.Sprintf(`You are a blockchain forensic analyst. Determine the relationship between the TARGET wallet and each KNOWN wallet of KOL "%s".

%s

For each known wallet, analyze:
1. Do they trade the same tokens?
2. Similar buy sizes?
3. Same DEX preferences?
4. Same fee/gas patterns (bot signatures)?
5. Active at similar times?
6. Direct transfers between them?
7. Funding amount correlation (sent X, received X minus 1-2%%)?

Return JSON:
{
  "linked_wallets": [
    {
      "address": "known_wallet_address",
      "chain": "chain",
      "relationship": "same_owner|wash|sniping|funding_hub|linked_entity|unrelated",
      "confidence": 0.0-1.0,
      "evidence": ["point 1","point 2"],
      "behavior_match": 0.0-1.0
    }
  ]
}
Return ONLY valid JSON.`, kol.Name, comparisonData.String())

		resp, err := e.callLLM(ctx, relationPrompt, "json")
		if err == nil {
			result.LLMCallsUsed++
			result.TotalTokensUsed += estimateTokens(relationPrompt, resp)
			var relResult struct {
				LinkedWallets []AILinkedWallet `json:"linked_wallets"`
			}
			json.Unmarshal(extractJSON(resp), &relResult)
			result.LinkedWallets = relResult.LinkedWallets

			for _, lw := range result.LinkedWallets {
				log.Info().Str("wallet", abbrev(lw.Address)).Str("rel", lw.Relationship).
					Float64("conf", lw.Confidence).Msg("🧠 relationship analyzed")
			}
		}
	}

	// ═══════════════════════════════════════════════════
	// CALL 3: FUNDING FLOW INTELLIGENCE
	// ═══════════════════════════════════════════════════
	fundingPrompt := fmt.Sprintf(`You are a blockchain forensic analyst specializing in fund flow analysis. Analyze how this wallet was funded.

KOL: %s
WALLET: %s (%s)

WALLET DATA:
- First active: %s
- Total transactions: %d
- Incoming transfers: %d
- Funding amounts: %s

KNOWN OBFUSCATION PATTERNS:
- FixedFloat: sends crypto on chain A, receives on chain B minus 0.5-2.5%% fee, 5-45 min delay
- Bridges: Wormhole, Stargate, Base Bridge - legitimate but used to hide origin chain
- Mixers: Tornado Cash, Railgun - strong evidence of intentional obfuscation
- Multi-hop: funds pass through 2-3 intermediate wallets before arriving
- Time delay: funds sent, then received hours/days later to break timing correlation
- Split sends: large amount broken into multiple smaller transfers

KOL'S OTHER WALLETS AND THEIR OUTGOING TRANSFERS:
%s

EXISTING WASH WALLET CANDIDATES:
%s

Return JSON:
{
  "primary_funding": "fixedfloat|bridge|cex|direct_transfer|mixer|multi_hop|unknown",
  "funding_chain": ["step 1: KOL wallet sent 24.5 SOL","step 2: FixedFloat processed","step 3: fresh wallet received 24.1 SOL"],
  "intentionally_obfuscated": true/false,
  "obfuscation_methods": ["fixedfloat","chain_hop","time_delay"],
  "estimated_total_funded": 0.0,
  "reasoning": "detailed explanation of the funding flow analysis"
}
Return ONLY valid JSON.`,
		kol.Name, abbrev(address), chain,
		targetProfile.FirstActive, targetProfile.TxCount,
		len(targetProfile.IncomingAmounts), formatAmounts(targetProfile.IncomingAmounts),
		knownSummary.String(), washContext.String())

	resp, err = e.callLLM(ctx, fundingPrompt, "json")
	if err == nil {
		result.LLMCallsUsed++
		result.TotalTokensUsed += estimateTokens(fundingPrompt, resp)
		var fa FundingAnalysis
		json.Unmarshal(extractJSON(resp), &fa)
		result.FundingAnalysis = &fa
		log.Info().Str("funding", fa.PrimaryFunding).Bool("obfuscated", fa.IntentionallyObfuscated).
			Msg("🧠 funding analysis complete")
	}

	// ═══════════════════════════════════════════════════
	// CALL 4: PREDICTIVE WALLET DISCOVERY
	// ═══════════════════════════════════════════════════
	predictPrompt := fmt.Sprintf(`You are a blockchain forensic analyst. Based on what you know about KOL "%s" and their wallets, predict what OTHER wallets they likely have that we haven't found yet.

KNOWN WALLETS:
%s

BEHAVIORAL PROFILE:
%s

RECENT SOCIAL POSTS:
%s

WASH CANDIDATES:
%s

Think about:
1. If they use FixedFloat, they probably have wallets on chains they send TO
2. If they snipe tokens, they need a dedicated sniping wallet (different from main)
3. If they have wallets on 2 chains, they probably have one on others too
4. If they use Trojan/BonkBot, there's a dedicated bot wallet
5. If they do paid promos, there's a separate "promotion dump" wallet
6. Looking at timing of buys vs posts - is there a pre-buy wallet?

Return JSON:
{
  "predicted_wallets": [
    {
      "description": "What this wallet does and why you think it exists",
      "chain": "solana|ethereum|base|bsc",
      "search_criteria": "How to find it: look for wallets that bought token X at time Y, or wallets funded by FixedFloat within Z hours of KOL's known outgoing tx",
      "confidence": 0.0-1.0,
      "priority": "high|medium|low"
    }
  ],
  "tokens_to_watch": ["tokens the KOL is likely to shill next based on their recent activity"],
  "recommendations": ["specific investigative steps to take"]
}
Return ONLY valid JSON.`,
		kol.Name, knownSummary.String(),
		formatBehaviorProfile(result.BehaviorProfile),
		postContext.String(), washContext.String())

	resp, err = e.callLLM(ctx, predictPrompt, "json")
	if err == nil {
		result.LLMCallsUsed++
		result.TotalTokensUsed += estimateTokens(predictPrompt, resp)
		var predResult struct {
			PredictedWallets []PredictedWallet `json:"predicted_wallets"`
			TokensToWatch    []string          `json:"tokens_to_watch"`
			Recommendations  []string          `json:"recommendations"`
		}
		json.Unmarshal(extractJSON(resp), &predResult)
		result.PredictedWallets = predResult.PredictedWallets
		result.TokensToWatch = predResult.TokensToWatch
		result.Recommendations = predResult.Recommendations

		log.Info().Int("predictions", len(result.PredictedWallets)).
			Int("tokens", len(result.TokensToWatch)).
			Msg("🧠 predictive analysis complete")
	}

	// ═══════════════════════════════════════════════════
	// CALL 5: RISK ASSESSMENT SYNTHESIS
	// ═══════════════════════════════════════════════════
	// This call synthesizes ALL previous findings into a final risk score
	riskPrompt := fmt.Sprintf(`You are a senior blockchain forensic analyst. Synthesize all findings into a final risk assessment for KOL "%s" based on wallet %s (%s).

BEHAVIORAL PROFILE:
%s

RELATIONSHIP ANALYSIS:
%s

FUNDING ANALYSIS:
%s

PREDICTED WALLETS:
%s

Give your final assessment:
{
  "overall_risk": "critical|high|medium|low|clean",
  "wash_trading_prob": 0.0-1.0,
  "insider_trading_prob": 0.0-1.0,
  "pre_buying_prob": 0.0-1.0,
  "pump_and_dump_prob": 0.0-1.0,
  "evidence": ["key evidence point 1","key evidence point 2"],
  "reasoning": "comprehensive 3-5 sentence risk assessment explaining your conclusion"
}

KEY RISK INDICATORS:
- FixedFloat funding + buying KOL's mentioned tokens = very high wash risk
- Fresh wallet + immediate trading = sniping or wash
- Cross-chain funding through bridges = moderate obfuscation
- Mixer usage = strong intent to hide
- Buying before social post = pre-buying / insider
- Multiple wallets buying same token = pump coordination
- Wallet only trades tokens the KOL mentions = wash confirmation

Return ONLY valid JSON.`,
		kol.Name, abbrev(address), chain,
		formatBehaviorProfile(result.BehaviorProfile),
		formatLinkedWallets(result.LinkedWallets),
		formatFundingAnalysis(result.FundingAnalysis),
		formatPredictedWallets(result.PredictedWallets))

	resp, err = e.callLLM(ctx, riskPrompt, "json")
	if err == nil {
		result.LLMCallsUsed++
		result.TotalTokensUsed += estimateTokens(riskPrompt, resp)
		var ra RiskAssessment
		json.Unmarshal(extractJSON(resp), &ra)
		result.RiskAssessment = &ra

		log.Info().Str("risk", ra.OverallRisk).Float64("wash_prob", ra.WashTradingProb).
			Float64("prebuy_prob", ra.PreBuyingProb).Msg("🧠 risk assessment complete")

		// Store alerts based on risk
		if ra.WashTradingProb >= 0.7 {
			e.store.InsertAlert(kolID, "ai_wash_confirmed", "critical",
				fmt.Sprintf("🧠 AI: High wash trading probability (%.0f%%) for %s's wallet %s",
					ra.WashTradingProb*100, kol.Name, abbrev(address)),
				ra.Reasoning, address, "")
		}
		if ra.PreBuyingProb >= 0.6 {
			e.store.InsertAlert(kolID, "ai_prebuy_detected", "critical",
				fmt.Sprintf("🧠 AI: Pre-buying detected (%.0f%%) for %s via wallet %s",
					ra.PreBuyingProb*100, kol.Name, abbrev(address)),
				ra.Reasoning, address, "")
		}
		if ra.OverallRisk == "critical" || ra.OverallRisk == "high" {
			e.store.InsertAlert(kolID, "ai_risk_assessment", "warning",
				fmt.Sprintf("🧠 AI Risk: %s for %s wallet %s",
					strings.ToUpper(ra.OverallRisk), kol.Name, abbrev(address)),
				ra.Reasoning, address, "")
		}
	}

	// ═══════════════════════════════════════════════════
	// POST-PROCESSING: Apply AI findings to database
	// ═══════════════════════════════════════════════════

	// Update wallet confidence based on AI assessment
	if result.RiskAssessment != nil {
		if result.BehaviorProfile != nil && result.BehaviorProfile.SimilarToKOLMain >= 0.8 {
			// High similarity to KOL main wallet - increase confidence
			e.store.UpsertWallet(kolID, address, chain,
				"ai:confirmed_kol", result.BehaviorProfile.SimilarToKOLMain,
				"ai_study")
		}
	}

	// Store linked wallet findings
	for _, lw := range result.LinkedWallets {
		if lw.Confidence >= 0.5 && lw.Relationship != "unrelated" {
			ch := config.Chain(lw.Chain)
			if ch == "" {
				ch = chain
			}
			e.store.UpsertWallet(kolID, lw.Address, ch,
				"ai:"+lw.Relationship, lw.Confidence,
				"ai_study:"+abbrev(address))
		}
	}

	// Store predicted wallet search criteria as alerts for manual follow-up
	for _, pw := range result.PredictedWallets {
		if pw.Confidence >= 0.5 && pw.Priority == "high" {
			e.store.InsertAlert(kolID, "ai_wallet_prediction", "info",
				fmt.Sprintf("🧠 AI predicts undiscovered wallet: %s", pw.Description),
				pw.SearchCriteria, "", "")
		}
	}

	log.Info().Str("wallet", abbrev(address)).Str("kol", kol.Name).
		Int("llm_calls", result.LLMCallsUsed).
		Int("tokens", result.TotalTokensUsed).
		Msg("🧠 AI wallet study complete")

	return result, nil
}

// ============================================================================
// HELPERS
// ============================================================================

func formatWalletSummary(w db.TrackedWallet, p WalletProfile) string {
	return fmt.Sprintf(`  %s (%s, %s, conf:%.0f%%)
    Txs:%d AvgBuy:$%.0f DEX:%s Fee:%.0f Hours:%s
    Outgoing amounts: %s
`,
		abbrev(w.Address), w.Chain, w.Label, w.Confidence*100,
		p.TxCount, p.AvgBuy, p.TopDEX, p.AvgFee, p.ActiveHours,
		formatAmounts(p.OutgoingAmounts))
}

func formatAmounts(amounts []float64) string {
	if len(amounts) == 0 {
		return "none"
	}
	var parts []string
	limit := len(amounts)
	if limit > 10 {
		limit = 10
	}
	for _, a := range amounts[:limit] {
		parts = append(parts, fmt.Sprintf("%.4f", a))
	}
	if len(amounts) > 10 {
		parts = append(parts, fmt.Sprintf("...+%d more", len(amounts)-10))
	}
	return strings.Join(parts, ", ")
}

func formatBehaviorProfile(bp *BehaviorProfile) string {
	if bp == nil {
		return "Not yet analyzed"
	}
	return fmt.Sprintf("Style: %s, Risk: %s, DEXs: %s, Bot: %s, Gas: %s, Hold: %s, Similarity: %.0f%%",
		bp.TradingStyle, bp.RiskTolerance, strings.Join(bp.PreferredDEXs, ","),
		bp.BotUsage, bp.GasStrategy, bp.TypicalHoldTime, bp.SimilarToKOLMain*100)
}

func formatLinkedWallets(lws []AILinkedWallet) string {
	if len(lws) == 0 {
		return "No linked wallets analyzed yet"
	}
	var parts []string
	for _, lw := range lws {
		parts = append(parts, fmt.Sprintf("%s: %s (%.0f%% conf, behavior match: %.0f%%)",
			abbrev(lw.Address), lw.Relationship, lw.Confidence*100, lw.BehaviorMatch*100))
	}
	return strings.Join(parts, "\n")
}

func formatFundingAnalysis(fa *FundingAnalysis) string {
	if fa == nil {
		return "Not yet analyzed"
	}
	return fmt.Sprintf("Funding: %s, Obfuscated: %v, Methods: %s\nReasoning: %s",
		fa.PrimaryFunding, fa.IntentionallyObfuscated,
		strings.Join(fa.ObfuscationMethods, ","), fa.Reasoning)
}

func formatPredictedWallets(pws []PredictedWallet) string {
	if len(pws) == 0 {
		return "No predictions"
	}
	var parts []string
	for _, pw := range pws {
		parts = append(parts, fmt.Sprintf("[%s, %.0f%%] %s", pw.Priority, pw.Confidence*100, pw.Description))
	}
	return strings.Join(parts, "\n")
}

// estimateTokens gives a rough token count for cost tracking.
// Average English word ≈ 1.3 tokens. Average char ≈ 0.25 tokens.
func estimateTokens(prompt, response string) int {
	return (len(prompt) + len(response)) / 4
}

// AIWalletStudyGeneric wraps AIWalletStudy to return interface{} for the scanner interface.
func (e *Engine) AIWalletStudyGeneric(ctx context.Context, kolID int64, address string, chain config.Chain) (interface{}, error) {
	return e.AIWalletStudy(ctx, kolID, address, chain)
}
