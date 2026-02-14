package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/kol-tracker/pkg/config"
)

const schema = `
CREATE TABLE IF NOT EXISTS kol_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    twitter_handle TEXT,
    telegram_channel TEXT,
    notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tracked_wallets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kol_id INTEGER REFERENCES kol_profiles(id),
    address TEXT NOT NULL,
    chain TEXT NOT NULL DEFAULT 'solana',
    label TEXT,
    confidence REAL DEFAULT 1.0,
    source TEXT,
    discovered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata TEXT DEFAULT '{}',
    UNIQUE(address, chain)
);

CREATE TABLE IF NOT EXISTS social_posts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kol_id INTEGER REFERENCES kol_profiles(id),
    platform TEXT NOT NULL,
    post_id TEXT NOT NULL,
    content TEXT NOT NULL,
    posted_at TIMESTAMP,
    extracted_tokens TEXT DEFAULT '[]',
    extracted_wallets TEXT DEFAULT '[]',
    extracted_links TEXT DEFAULT '[]',
    processed BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(platform, post_id)
);

CREATE TABLE IF NOT EXISTS token_mentions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kol_id INTEGER REFERENCES kol_profiles(id),
    post_id INTEGER REFERENCES social_posts(id),
    token_address TEXT,
    token_symbol TEXT,
    chain TEXT DEFAULT 'solana',
    mentioned_at TIMESTAMP,
    pre_buy_detected BOOLEAN DEFAULT FALSE,
    post_buy_detected BOOLEAN DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS wallet_transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    wallet_id INTEGER REFERENCES tracked_wallets(id),
    tx_hash TEXT NOT NULL,
    chain TEXT NOT NULL,
    tx_type TEXT,
    token_address TEXT,
    token_symbol TEXT,
    amount_token REAL,
    amount_usd REAL,
    from_address TEXT,
    to_address TEXT,
    timestamp TIMESTAMP,
    block_number INTEGER,
    platform TEXT,
    priority_fee REAL DEFAULT 0,
    metadata TEXT DEFAULT '{}',
    UNIQUE(tx_hash, chain)
);

CREATE TABLE IF NOT EXISTS wash_wallet_candidates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    address TEXT NOT NULL,
    chain TEXT NOT NULL DEFAULT 'solana',
    funded_by TEXT,
    funding_source_type TEXT,
    funding_amount REAL,
    funding_token TEXT,
    funding_tx TEXT,
    first_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    bought_same_token BOOLEAN DEFAULT FALSE,
    timing_match BOOLEAN DEFAULT FALSE,
    amount_pattern_match BOOLEAN DEFAULT FALSE,
    bot_signature_match BOOLEAN DEFAULT FALSE,
    confidence_score REAL DEFAULT 0.0,
    linked_kol_id INTEGER REFERENCES kol_profiles(id),
    status TEXT DEFAULT 'candidate',
    notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(address, chain)
);

CREATE TABLE IF NOT EXISTS trading_patterns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kol_id INTEGER REFERENCES kol_profiles(id),
    pattern_type TEXT NOT NULL,
    pattern_data TEXT NOT NULL,
    sample_count INTEGER DEFAULT 0,
    last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(kol_id, pattern_type)
);

CREATE TABLE IF NOT EXISTS funding_flow_matches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_tx TEXT,
    source_chain TEXT,
    source_amount REAL,
    source_token TEXT,
    dest_address TEXT,
    dest_chain TEXT,
    dest_amount REAL,
    dest_token TEXT,
    service TEXT,
    amount_diff_pct REAL,
    time_diff_seconds INTEGER,
    match_confidence REAL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS alerts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kol_id INTEGER REFERENCES kol_profiles(id),
    alert_type TEXT NOT NULL,
    severity TEXT DEFAULT 'info',
    title TEXT NOT NULL,
    description TEXT,
    related_wallet TEXT,
    related_token TEXT,
    metadata TEXT DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_wallet_addr ON tracked_wallets(address);
CREATE INDEX IF NOT EXISTS idx_wallet_chain ON tracked_wallets(chain);
CREATE INDEX IF NOT EXISTS idx_wash_addr ON wash_wallet_candidates(address);
CREATE INDEX IF NOT EXISTS idx_wash_score ON wash_wallet_candidates(confidence_score);
CREATE INDEX IF NOT EXISTS idx_tx_wallet ON wallet_transactions(wallet_id);
CREATE INDEX IF NOT EXISTS idx_tx_token ON wallet_transactions(token_address);
CREATE INDEX IF NOT EXISTS idx_tx_time ON wallet_transactions(timestamp);
CREATE INDEX IF NOT EXISTS idx_mention_token ON token_mentions(token_address);
CREATE INDEX IF NOT EXISTS idx_mention_kol ON token_mentions(kol_id);
CREATE INDEX IF NOT EXISTS idx_post_kol ON social_posts(kol_id);
CREATE INDEX IF NOT EXISTS idx_alert_time ON alerts(created_at);
`

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// ---- KOL Profiles ----

func (s *Store) UpsertKOL(name, twitterHandle, telegramChannel string) (int64, error) {
	res, err := s.db.Exec(`
		INSERT INTO kol_profiles (name, twitter_handle, telegram_channel)
		VALUES (?, ?, ?)
		ON CONFLICT DO UPDATE SET name=excluded.name`,
		name, twitterHandle, telegramChannel)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetKOLs() ([]KOLProfile, error) {
	rows, err := s.db.Query("SELECT id, name, twitter_handle, telegram_channel, COALESCE(notes,''), created_at FROM kol_profiles")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var kols []KOLProfile
	for rows.Next() {
		var k KOLProfile
		if err := rows.Scan(&k.ID, &k.Name, &k.TwitterHandle, &k.TelegramChannel, &k.Notes, &k.CreatedAt); err != nil {
			continue
		}
		kols = append(kols, k)
	}
	return kols, nil
}

func (s *Store) GetKOLByHandle(handle string) (*KOLProfile, error) {
	var k KOLProfile
	err := s.db.QueryRow(
		"SELECT id, name, twitter_handle, telegram_channel, COALESCE(notes,''), created_at FROM kol_profiles WHERE twitter_handle=? OR telegram_channel=?",
		handle, handle).Scan(&k.ID, &k.Name, &k.TwitterHandle, &k.TelegramChannel, &k.Notes, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// ---- Tracked Wallets ----

func (s *Store) UpsertWallet(kolID int64, address string, chain config.Chain, label string, confidence float64, source string) (int64, error) {
	res, err := s.db.Exec(`
		INSERT INTO tracked_wallets (kol_id, address, chain, label, confidence, source)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(address, chain) DO UPDATE SET
			confidence = MAX(tracked_wallets.confidence, excluded.confidence),
			label = CASE WHEN excluded.confidence > tracked_wallets.confidence THEN excluded.label ELSE tracked_wallets.label END`,
		kolID, address, string(chain), label, confidence, source)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetWalletsForKOL(kolID int64) ([]TrackedWallet, error) {
	rows, err := s.db.Query(`
		SELECT id, kol_id, address, chain, COALESCE(label,''), confidence, COALESCE(source,''), discovered_at, COALESCE(metadata,'{}')
		FROM tracked_wallets WHERE kol_id=? ORDER BY confidence DESC`, kolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallets []TrackedWallet
	for rows.Next() {
		var w TrackedWallet
		var chain string
		if err := rows.Scan(&w.ID, &w.KOLID, &w.Address, &chain, &w.Label, &w.Confidence, &w.Source, &w.DiscoveredAt, &w.Metadata); err != nil {
			continue
		}
		w.Chain = config.Chain(chain)
		wallets = append(wallets, w)
	}
	return wallets, nil
}

func (s *Store) GetAllTrackedAddresses() ([]TrackedWallet, error) {
	rows, err := s.db.Query(`SELECT id, kol_id, address, chain, COALESCE(label,''), confidence, COALESCE(source,''), discovered_at, COALESCE(metadata,'{}') FROM tracked_wallets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallets []TrackedWallet
	for rows.Next() {
		var w TrackedWallet
		var chain string
		if err := rows.Scan(&w.ID, &w.KOLID, &w.Address, &chain, &w.Label, &w.Confidence, &w.Source, &w.DiscoveredAt, &w.Metadata); err != nil {
			continue
		}
		w.Chain = config.Chain(chain)
		wallets = append(wallets, w)
	}
	return wallets, nil
}

func (s *Store) GetWalletByAddress(address string, chain config.Chain) (*TrackedWallet, error) {
	var w TrackedWallet
	var ch string
	err := s.db.QueryRow(`SELECT id, kol_id, address, chain, COALESCE(label,''), confidence FROM tracked_wallets WHERE address=? AND chain=?`,
		address, string(chain)).Scan(&w.ID, &w.KOLID, &w.Address, &ch, &w.Label, &w.Confidence)
	if err != nil {
		return nil, err
	}
	w.Chain = config.Chain(ch)
	return &w, nil
}

// ---- Social Posts ----

func (s *Store) InsertPost(kolID int64, platform, postID, content string, postedAt time.Time, tokens, wallets, links []string) (int64, error) {
	tokensJSON, _ := json.Marshal(tokens)
	walletsJSON, _ := json.Marshal(wallets)
	linksJSON, _ := json.Marshal(links)

	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO social_posts (kol_id, platform, post_id, content, posted_at, extracted_tokens, extracted_wallets, extracted_links)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		kolID, platform, postID, content, postedAt, string(tokensJSON), string(walletsJSON), string(linksJSON))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetUnprocessedPosts() ([]SocialPost, error) {
	rows, err := s.db.Query(`SELECT id, kol_id, platform, post_id, content, posted_at, extracted_tokens, extracted_wallets, extracted_links FROM social_posts WHERE processed=FALSE ORDER BY posted_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []SocialPost
	for rows.Next() {
		var p SocialPost
		if err := rows.Scan(&p.ID, &p.KOLID, &p.Platform, &p.PostID, &p.Content, &p.PostedAt, &p.ExtractedTokens, &p.ExtractedWallets, &p.ExtractedLinks); err != nil {
			continue
		}
		posts = append(posts, p)
	}
	return posts, nil
}

func (s *Store) MarkPostProcessed(id int64) error {
	_, err := s.db.Exec("UPDATE social_posts SET processed=TRUE WHERE id=?", id)
	return err
}

// ---- Token Mentions ----

func (s *Store) InsertTokenMention(kolID, postID int64, tokenAddr, tokenSymbol string, chain config.Chain, mentionedAt time.Time) error {
	_, err := s.db.Exec(`INSERT INTO token_mentions (kol_id, post_id, token_address, token_symbol, chain, mentioned_at) VALUES (?,?,?,?,?,?)`,
		kolID, postID, tokenAddr, tokenSymbol, string(chain), mentionedAt)
	return err
}

func (s *Store) GetRecentTokenMentions(hours int) ([]TokenMention, error) {
	rows, err := s.db.Query(`
		SELECT id, kol_id, post_id, COALESCE(token_address,''), COALESCE(token_symbol,''), chain, mentioned_at
		FROM token_mentions WHERE mentioned_at > datetime('now', ?)
		ORDER BY mentioned_at DESC`, fmt.Sprintf("-%d hours", hours))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mentions []TokenMention
	for rows.Next() {
		var m TokenMention
		var chain string
		if err := rows.Scan(&m.ID, &m.KOLID, &m.PostID, &m.TokenAddress, &m.TokenSymbol, &chain, &m.MentionedAt); err != nil {
			continue
		}
		m.Chain = config.Chain(chain)
		mentions = append(mentions, m)
	}
	return mentions, nil
}

// ---- Wallet Transactions ----

func (s *Store) InsertTransaction(tx WalletTransaction) error {
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO wallet_transactions
		(wallet_id, tx_hash, chain, tx_type, token_address, token_symbol, amount_token, amount_usd,
		 from_address, to_address, timestamp, block_number, platform, priority_fee, metadata)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		tx.WalletID, tx.TxHash, string(tx.Chain), tx.TxType, tx.TokenAddress, tx.TokenSymbol,
		tx.AmountToken, tx.AmountUSD, tx.FromAddress, tx.ToAddress, tx.Timestamp,
		tx.BlockNumber, tx.Platform, tx.PriorityFee, tx.Metadata)
	return err
}

func (s *Store) GetTransactionsForWallet(walletID int64, limit int) ([]WalletTransaction, error) {
	rows, err := s.db.Query(`
		SELECT id, wallet_id, tx_hash, chain, COALESCE(tx_type,''), COALESCE(token_address,''),
			   COALESCE(token_symbol,''), amount_token, amount_usd, COALESCE(from_address,''),
			   COALESCE(to_address,''), timestamp, block_number, COALESCE(platform,''), priority_fee
		FROM wallet_transactions WHERE wallet_id=? ORDER BY timestamp DESC LIMIT ?`, walletID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []WalletTransaction
	for rows.Next() {
		var t WalletTransaction
		var chain string
		if err := rows.Scan(&t.ID, &t.WalletID, &t.TxHash, &chain, &t.TxType, &t.TokenAddress,
			&t.TokenSymbol, &t.AmountToken, &t.AmountUSD, &t.FromAddress, &t.ToAddress,
			&t.Timestamp, &t.BlockNumber, &t.Platform, &t.PriorityFee); err != nil {
			continue
		}
		t.Chain = config.Chain(chain)
		txs = append(txs, t)
	}
	return txs, nil
}

func (s *Store) GetBuyTransactionsForAddress(address string) ([]WalletTransaction, error) {
	rows, err := s.db.Query(`
		SELECT wt.id, wt.wallet_id, wt.tx_hash, wt.chain, wt.tx_type, COALESCE(wt.token_address,''),
			   COALESCE(wt.token_symbol,''), wt.amount_token, wt.amount_usd, wt.timestamp, COALESCE(wt.platform,''), wt.priority_fee
		FROM wallet_transactions wt
		JOIN tracked_wallets tw ON wt.wallet_id = tw.id
		WHERE tw.address=? AND wt.tx_type='swap_buy'
		ORDER BY wt.timestamp DESC`, address)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []WalletTransaction
	for rows.Next() {
		var t WalletTransaction
		var chain string
		if err := rows.Scan(&t.ID, &t.WalletID, &t.TxHash, &chain, &t.TxType, &t.TokenAddress,
			&t.TokenSymbol, &t.AmountToken, &t.AmountUSD, &t.Timestamp, &t.Platform, &t.PriorityFee); err != nil {
			continue
		}
		t.Chain = config.Chain(chain)
		txs = append(txs, t)
	}
	return txs, nil
}

// ---- Wash Wallet Candidates ----

func (s *Store) UpsertWashCandidate(wc WashWalletCandidate) error {
	_, err := s.db.Exec(`
		INSERT INTO wash_wallet_candidates
		(address, chain, funded_by, funding_source_type, funding_amount, funding_token, funding_tx, confidence_score, linked_kol_id, notes)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(address, chain) DO UPDATE SET
			confidence_score = MAX(wash_wallet_candidates.confidence_score, excluded.confidence_score),
			funding_source_type = COALESCE(excluded.funding_source_type, wash_wallet_candidates.funding_source_type)`,
		wc.Address, string(wc.Chain), wc.FundedBy, wc.FundingSourceType, wc.FundingAmount,
		wc.FundingToken, wc.FundingTx, wc.ConfidenceScore, wc.LinkedKOLID, wc.Notes)
	return err
}

func (s *Store) UpdateWashScore(address string, chain config.Chain, score float64, signals map[string]bool) error {
	setClauses := "confidence_score=?"
	args := []interface{}{score}

	for k, v := range signals {
		switch k {
		case "bought_same_token":
			setClauses += ", bought_same_token=?"
			args = append(args, v)
		case "timing_match":
			setClauses += ", timing_match=?"
			args = append(args, v)
		case "amount_pattern_match":
			setClauses += ", amount_pattern_match=?"
			args = append(args, v)
		case "bot_signature_match":
			setClauses += ", bot_signature_match=?"
			args = append(args, v)
		}
	}

	args = append(args, address, string(chain))
	_, err := s.db.Exec(fmt.Sprintf("UPDATE wash_wallet_candidates SET %s WHERE address=? AND chain=?", setClauses), args...)
	return err
}

func (s *Store) GetWashCandidates(minScore float64) ([]WashWalletCandidate, error) {
	rows, err := s.db.Query(`
		SELECT id, address, chain, COALESCE(funded_by,''), COALESCE(funding_source_type,'unknown'),
			   funding_amount, COALESCE(funding_token,''), COALESCE(funding_tx,''), first_seen,
			   bought_same_token, timing_match, amount_pattern_match, bot_signature_match,
			   confidence_score, linked_kol_id, status, COALESCE(notes,'')
		FROM wash_wallet_candidates
		WHERE confidence_score >= ? AND status='candidate'
		ORDER BY confidence_score DESC`, minScore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []WashWalletCandidate
	for rows.Next() {
		var c WashWalletCandidate
		var chain string
		if err := rows.Scan(&c.ID, &c.Address, &chain, &c.FundedBy, &c.FundingSourceType,
			&c.FundingAmount, &c.FundingToken, &c.FundingTx, &c.FirstSeen,
			&c.BoughtSameToken, &c.TimingMatch, &c.AmountPatternMatch, &c.BotSignatureMatch,
			&c.ConfidenceScore, &c.LinkedKOLID, &c.Status, &c.Notes); err != nil {
			continue
		}
		c.Chain = config.Chain(chain)
		candidates = append(candidates, c)
	}
	return candidates, nil
}

// ---- Trading Patterns ----

func (s *Store) UpsertPattern(kolID int64, patternType string, data interface{}, sampleCount int) error {
	jsonData, _ := json.Marshal(data)
	_, err := s.db.Exec(`
		INSERT INTO trading_patterns (kol_id, pattern_type, pattern_data, sample_count, last_updated)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(kol_id, pattern_type) DO UPDATE SET
			pattern_data=excluded.pattern_data, sample_count=excluded.sample_count, last_updated=CURRENT_TIMESTAMP`,
		kolID, patternType, string(jsonData), sampleCount)
	return err
}

func (s *Store) GetPatternsForKOL(kolID int64) ([]TradingPattern, error) {
	rows, err := s.db.Query("SELECT id, kol_id, pattern_type, pattern_data, sample_count, last_updated FROM trading_patterns WHERE kol_id=?", kolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patterns []TradingPattern
	for rows.Next() {
		var p TradingPattern
		if err := rows.Scan(&p.ID, &p.KOLID, &p.PatternType, &p.PatternData, &p.SampleCount, &p.LastUpdated); err != nil {
			continue
		}
		patterns = append(patterns, p)
	}
	return patterns, nil
}

// ---- Alerts ----

func (s *Store) InsertAlert(kolID int64, alertType, severity, title, description, wallet, token string) error {
	_, err := s.db.Exec(`INSERT INTO alerts (kol_id, alert_type, severity, title, description, related_wallet, related_token) VALUES (?,?,?,?,?,?,?)`,
		kolID, alertType, severity, title, description, wallet, token)
	return err
}

func (s *Store) GetRecentAlerts(limit int) ([]Alert, error) {
	rows, err := s.db.Query(`SELECT id, kol_id, alert_type, severity, title, COALESCE(description,''), COALESCE(related_wallet,''), COALESCE(related_token,''), created_at FROM alerts ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.KOLID, &a.AlertType, &a.Severity, &a.Title, &a.Description, &a.RelatedWallet, &a.RelatedToken, &a.CreatedAt); err != nil {
			continue
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

// ---- Funding Flow Matches ----

func (s *Store) InsertFundingMatch(fm FundingFlowMatch) error {
	_, err := s.db.Exec(`INSERT INTO funding_flow_matches (source_tx, source_chain, source_amount, source_token, dest_address, dest_chain, dest_amount, dest_token, service, amount_diff_pct, time_diff_seconds, match_confidence) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		fm.SourceTx, string(fm.SourceChain), fm.SourceAmount, fm.SourceToken, fm.DestAddress,
		string(fm.DestChain), fm.DestAmount, fm.DestToken, fm.Service, fm.AmountDiffPct,
		fm.TimeDiffSeconds, fm.MatchConfidence)
	return err
}

func (s *Store) GetFundingMatches(limit int) ([]FundingFlowMatch, error) {
	rows, err := s.db.Query(`SELECT id, source_tx, source_chain, source_amount, source_token,
		dest_address, dest_chain, dest_amount, dest_token, service,
		amount_diff_pct, time_diff_seconds, match_confidence, created_at
		FROM funding_flow_matches ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []FundingFlowMatch
	for rows.Next() {
		var fm FundingFlowMatch
		err := rows.Scan(&fm.ID, &fm.SourceTx, &fm.SourceChain, &fm.SourceAmount, &fm.SourceToken,
			&fm.DestAddress, &fm.DestChain, &fm.DestAmount, &fm.DestToken,
			&fm.Service, &fm.AmountDiffPct, &fm.TimeDiffSeconds, &fm.MatchConfidence, &fm.CreatedAt)
		if err != nil {
			continue
		}
		matches = append(matches, fm)
	}
	return matches, nil
}

// ---- Stats ----

func (s *Store) GetStats() (map[string]int64, error) {
	stats := map[string]int64{}
	tables := []string{"kol_profiles", "tracked_wallets", "social_posts", "token_mentions",
		"wallet_transactions", "wash_wallet_candidates", "alerts"}

	for _, t := range tables {
		var count int64
		if err := s.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", t)).Scan(&count); err == nil {
			stats[t] = count
		}
	}

	// High-confidence wash candidates
	var wc int64
	s.db.QueryRow("SELECT COUNT(*) FROM wash_wallet_candidates WHERE confidence_score >= 0.5").Scan(&wc)
	stats["high_confidence_wash"] = wc

	return stats, nil
}

func (s *Store) GetKOLByID(id int64) (*KOLProfile, error) {
	var k KOLProfile
	err := s.db.QueryRow(
		"SELECT id, name, COALESCE(twitter_handle,''), COALESCE(telegram_channel,''), COALESCE(notes,''), created_at FROM kol_profiles WHERE id=?",
		id).Scan(&k.ID, &k.Name, &k.TwitterHandle, &k.TelegramChannel, &k.Notes, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (s *Store) GetPostsForKOL(kolID int64, limit int) ([]SocialPost, error) {
	rows, err := s.db.Query(`SELECT id, kol_id, platform, post_id, content, posted_at, extracted_tokens, extracted_wallets, extracted_links, processed FROM social_posts WHERE kol_id=? ORDER BY posted_at DESC LIMIT ?`, kolID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var posts []SocialPost
	for rows.Next() {
		var p SocialPost
		if err := rows.Scan(&p.ID, &p.KOLID, &p.Platform, &p.PostID, &p.Content, &p.PostedAt, &p.ExtractedTokens, &p.ExtractedWallets, &p.ExtractedLinks, &p.Processed); err != nil {
			continue
		}
		posts = append(posts, p)
	}
	return posts, nil
}

func (s *Store) GetWashCandidatesForKOL(kolID int64) ([]WashWalletCandidate, error) {
	rows, err := s.db.Query(`SELECT id, address, chain, COALESCE(funded_by,''), COALESCE(funding_source_type,'unknown'), funding_amount, COALESCE(funding_token,''), COALESCE(funding_tx,''), first_seen, bought_same_token, timing_match, amount_pattern_match, bot_signature_match, confidence_score, linked_kol_id, status, COALESCE(notes,'') FROM wash_wallet_candidates WHERE linked_kol_id=? ORDER BY confidence_score DESC`, kolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candidates []WashWalletCandidate
	for rows.Next() {
		var c WashWalletCandidate
		var chain string
		if err := rows.Scan(&c.ID, &c.Address, &chain, &c.FundedBy, &c.FundingSourceType, &c.FundingAmount, &c.FundingToken, &c.FundingTx, &c.FirstSeen, &c.BoughtSameToken, &c.TimingMatch, &c.AmountPatternMatch, &c.BotSignatureMatch, &c.ConfidenceScore, &c.LinkedKOLID, &c.Status, &c.Notes); err != nil {
			continue
		}
		c.Chain = config.Chain(chain)
		candidates = append(candidates, c)
	}
	return candidates, nil
}
