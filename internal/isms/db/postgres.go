// Package db provides PostgreSQL storage for the ISMS collaboration layer.
package db

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps the PostgreSQL connection pool.
type DB struct {
	pool          *pgxpool.Pool
	encryptionKey string // derived from ISMS_SECRET, used for encrypting OIDC secrets at rest
}

// SetEncryptionKey sets the key used for encrypting secrets at rest (e.g. OIDC client secrets).
func (d *DB) SetEncryptionKey(key string) {
	d.encryptionKey = key
}

// New creates a new DB from a connection string.
func New(ctx context.Context, connStr string) (*DB, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Close closes the connection pool.
func (d *DB) Close() {
	d.pool.Close()
}

// Pool returns the underlying connection pool for direct queries.
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

// Healthcheck executes a tiny SELECT against a real table to confirm the DB
// is not just reachable on TCP but actually queryable (statement engine alive,
// no max-connections deadlock, schema migrated, RLS not blocking everything).
// Ping can succeed against a database that's wedged — this can't. Uses count(*)
// so it always returns a row even on a fresh empty database.
func (d *DB) Healthcheck(ctx context.Context) error {
	var n int
	return d.pool.QueryRow(ctx, `SELECT count(*) FROM organizations`).Scan(&n)
}

// NextIdentifier atomically allocates the next identifier for an entity type within an org.
// Returns formatted identifier like "RISK-1", "ASSET-42", etc.
func (d *DB) NextIdentifier(ctx context.Context, orgID int, entityType string) (string, error) {
	prefix := map[string]string{
		"risk": "RISK", "asset": "ASSET", "supplier": "SUPPLIER",
		"system": "SYSTEM", "legal_requirement": "LEGAL", "program": "PROG",
		"incident": "INC", "change_request": "CR", "task": "TASK", "corrective_action": "CA",
		"objective": "OBJ", "audit": "AUDIT", "audit_finding": "FIND",
	}[entityType]
	if prefix == "" {
		prefix = strings.ToUpper(entityType)
	}

	var seq int
	err := d.pool.QueryRow(ctx, `
		INSERT INTO identifier_sequences (organization_id, entity_type, next_value)
		VALUES ($1, $2, 1)
		ON CONFLICT (organization_id, entity_type) DO UPDATE SET next_value = identifier_sequences.next_value + 1
		RETURNING next_value
	`, orgID, entityType).Scan(&seq)
	if err != nil {
		return "", fmt.Errorf("allocate identifier: %w", err)
	}
	return fmt.Sprintf("%s-%d", prefix, seq), nil
}

// WithOrgTx runs fn inside a transaction with RLS context set via SET LOCAL.
// The org context is automatically cleared when the transaction ends.
// Use this for operations that need guaranteed RLS isolation.
func (d *DB) WithOrgTx(ctx context.Context, orgID int, fn func(ctx context.Context, tx pgx.Tx) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.current_org_id = '%d'", orgID)); err != nil {
		return fmt.Errorf("set org context: %w", err)
	}

	if err := fn(ctx, tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// CleanupExpired removes expired login attempts, OIDC sessions, and JWT blocklist entries.
func (d *DB) CleanupExpired(ctx context.Context) {
	_, _ = d.pool.Exec(ctx, `DELETE FROM login_attempts WHERE expires_at < now()`)
	_, _ = d.pool.Exec(ctx, `DELETE FROM oidc_sessions WHERE expires_at < now()`)
	_, _ = d.pool.Exec(ctx, `DELETE FROM jwt_blocklist WHERE expires_at < now()`)
}

// --- Migrations ---

// Migrate runs all pending SQL migrations from the given directory.
func (d *DB) Migrate(ctx context.Context, migrationsDir string) error {
	// Ensure schema_migrations table exists.
	if _, err := d.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	// Get applied versions.
	rows, err := d.pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}

	// Read migration files.
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	// Apply pending migrations.
	count := 0
	for _, name := range files {
		if applied[name] {
			continue
		}

		sql, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}

		fmt.Printf("  Applying %s...\n", name)
		if _, err := d.pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		if _, err := d.pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			return fmt.Errorf("recording %s: %w", name, err)
		}
		count++
	}

	if count == 0 {
		fmt.Println("  No pending migrations.")
	} else {
		fmt.Printf("  Applied %d migration(s).\n", count)
	}
	return nil
}

// MigrateFS runs migrations from an embedded filesystem.
func (d *DB) MigrateFS(ctx context.Context, fsys fs.FS, dir string) error {
	// Ensure schema_migrations table exists.
	if _, err := d.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	// Get applied versions.
	rows, err := d.pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}

	// Read migration files from embedded FS.
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("reading migrations: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	count := 0
	for _, name := range files {
		if applied[name] {
			continue
		}

		sql, err := fs.ReadFile(fsys, dir+"/"+name)
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}

		fmt.Printf("  Applying %s...\n", name)
		if _, err := d.pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		if _, err := d.pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			return fmt.Errorf("recording %s: %w", name, err)
		}
		count++
	}

	if count == 0 {
		fmt.Println("  No pending migrations.")
	} else {
		fmt.Printf("  Applied %d migration(s).\n", count)
	}
	return nil
}

// --- Reviews ---

type Review struct {
	ID             int       `json:"id"`
	OrganizationID int       `json:"organization_id"`
	DocumentID     string    `json:"document_id"`
	DocumentType   string    `json:"document_type"`
	Title          string    `json:"title"`
	Version        string    `json:"version"`
	CommitHash     string    `json:"commit_hash,omitempty"`
	SentHead       string    `json:"sent_head,omitempty"`
	MergeCommit    string    `json:"merge_commit,omitempty"`
	Round          int       `json:"round"`
	RequestedBy    string    `json:"requested_by"`
	Message        string    `json:"message"`
	Status         string    `json:"status"`
	CreatedAt      Epoch `json:"created_at"`
	UpdatedAt      Epoch `json:"updated_at"`
	// Computed fields for API
	CommentCount int `json:"comment_count,omitempty"`
	OpenComments int `json:"open_comments,omitempty"`
}

func (d *DB) CreateReview(ctx context.Context, orgID int, r *Review) error {
	r.OrganizationID = orgID
	err := d.pool.QueryRow(ctx, `
		INSERT INTO reviews (organization_id, document_id, document_type, title, version, commit_hash, sent_head, requested_by_id, message, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, (SELECT id FROM users WHERE email = $8), $9, $10)
		RETURNING id, round, created_at, updated_at
	`, orgID, r.DocumentID, r.DocumentType, r.Title, r.Version, r.CommitHash, r.SentHead, r.RequestedBy, r.Message, r.Status,
	).Scan(&r.ID, &r.Round, &r.CreatedAt, &r.UpdatedAt)
	return err
}

func (d *DB) GetReview(ctx context.Context, orgID int, id int) (*Review, error) {
	var r Review
	err := d.pool.QueryRow(ctx, `
		SELECT r.id, r.organization_id, r.document_id, r.document_type, r.title, r.version, COALESCE(r.commit_hash, ''), COALESCE(r.sent_head, ''), COALESCE(r.merge_commit, ''), r.round,
			(SELECT email FROM users WHERE id = r.requested_by_id), COALESCE(r.message, ''), r.status, r.created_at, r.updated_at,
			(SELECT COUNT(*) FROM comments c WHERE c.review_id = r.id),
			(SELECT COUNT(*) FROM comments c WHERE c.review_id = r.id AND c.status = 'open')
		FROM reviews r WHERE r.id = $1 AND r.organization_id = $2
	`, id, orgID).Scan(&r.ID, &r.OrganizationID, &r.DocumentID, &r.DocumentType, &r.Title, &r.Version, &r.CommitHash, &r.SentHead, &r.MergeCommit, &r.Round, &r.RequestedBy, &r.Message, &r.Status, &r.CreatedAt, &r.UpdatedAt, &r.CommentCount, &r.OpenComments)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) ListReviews(ctx context.Context, orgID int, status string, limit int) ([]Review, error) {
	query := `
		SELECT r.id, r.organization_id, r.document_id, r.document_type, r.title, r.version, COALESCE(r.commit_hash, ''), COALESCE(r.sent_head, ''), COALESCE(r.merge_commit, ''), r.round,
			(SELECT email FROM users WHERE id = r.requested_by_id), COALESCE(r.message, ''), r.status, r.created_at, r.updated_at,
			(SELECT COUNT(*) FROM comments c WHERE c.review_id = r.id),
			(SELECT COUNT(*) FROM comments c WHERE c.review_id = r.id AND c.status = 'open')
		FROM reviews r WHERE r.organization_id = $1`
	args := []interface{}{orgID}
	if status != "" {
		query += ` AND r.status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY r.updated_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reviews := []Review{}
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ID, &r.OrganizationID, &r.DocumentID, &r.DocumentType, &r.Title, &r.Version, &r.CommitHash, &r.SentHead, &r.MergeCommit, &r.Round, &r.RequestedBy, &r.Message, &r.Status, &r.CreatedAt, &r.UpdatedAt, &r.CommentCount, &r.OpenComments); err != nil {
			return nil, err
		}
		reviews = append(reviews, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return reviews, nil
}

// ReviewListParams specifies filtering, sorting, and pagination for reviews.
// Phase is a meta-filter that maps to a set of statuses:
//   "open"   → open / changes_requested / approved
//   "closed" → merged / closed
// Use Status for an exact single-status match instead.
type ReviewListParams struct {
	Page   int
	Limit  int
	Sort   string
	Search string
	Status string
	Phase  string
}

var reviewSortable = map[string]string{
	"title":    "r.title",
	"document": "r.document_id",
	"status":   "r.status",
	"created":  "r.created_at",
	"updated":  "r.updated_at",
}

const reviewSelectCols = `r.id, r.organization_id, r.document_id, r.document_type, r.title, r.version,
	COALESCE(r.commit_hash, ''), COALESCE(r.sent_head, ''), COALESCE(r.merge_commit, ''), r.round,
	(SELECT email FROM users WHERE id = r.requested_by_id),
	COALESCE(r.message, ''), r.status, r.created_at, r.updated_at,
	(SELECT COUNT(*) FROM comments c WHERE c.review_id = r.id),
	(SELECT COUNT(*) FROM comments c WHERE c.review_id = r.id AND c.status = 'open')`

// ReviewStats returns counts per status for the org.
func (d *DB) ReviewStats(ctx context.Context, orgID int) (map[string]int, error) {
	rows, err := d.pool.Query(ctx, `SELECT status, COUNT(*) FROM reviews WHERE organization_id = $1 GROUP BY status`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		stats[s] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}

// PaginatedReviews returns a filtered/sorted/paginated slice of reviews plus total count.
func (d *DB) PaginatedReviews(ctx context.Context, orgID int, p ReviewListParams) ([]Review, int, error) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Limit < 1 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}

	where := ` WHERE r.organization_id = $1`
	args := []interface{}{orgID}
	idx := 2
	if p.Search != "" {
		where += fmt.Sprintf(` AND (r.title ILIKE $%d OR r.document_id ILIKE $%d OR r.message ILIKE $%d)`, idx, idx, idx)
		args = append(args, "%"+p.Search+"%")
		idx++
	}
	if p.Status != "" {
		where += fmt.Sprintf(` AND r.status = $%d`, idx)
		args = append(args, p.Status)
		idx++
	} else if p.Phase == "open" {
		where += ` AND r.status IN ('open','changes_requested','approved')`
	} else if p.Phase == "closed" {
		where += ` AND r.status IN ('merged','closed')`
	}

	var total int
	if err := d.pool.QueryRow(ctx, `SELECT COUNT(*) FROM reviews r`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortDir := "DESC"
	sortKey := strings.TrimPrefix(p.Sort, "-")
	if !strings.HasPrefix(p.Sort, "-") && p.Sort != "" {
		sortDir = "ASC"
	}
	sortField, ok := reviewSortable[sortKey]
	if !ok {
		sortField = "r.updated_at"
		sortDir = "DESC"
	}

	offset := (p.Page - 1) * p.Limit
	args = append(args, p.Limit, offset)
	limitIdx, offsetIdx := idx, idx+1

	q := `SELECT ` + reviewSelectCols + ` FROM reviews r` + where +
		` ORDER BY ` + sortField + ` ` + sortDir + `, r.id DESC` +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, limitIdx, offsetIdx)

	rows, err := d.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	reviews := []Review{}
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ID, &r.OrganizationID, &r.DocumentID, &r.DocumentType, &r.Title, &r.Version,
			&r.CommitHash, &r.SentHead, &r.MergeCommit, &r.Round,
			&r.RequestedBy, &r.Message, &r.Status, &r.CreatedAt, &r.UpdatedAt,
			&r.CommentCount, &r.OpenComments); err != nil {
			return nil, 0, err
		}
		reviews = append(reviews, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return reviews, total, nil
}

// IsUserAgent checks if a user is an agent account.
func (d *DB) IsUserAgent(ctx context.Context, email string) bool {
	var isAgent bool
	err := d.pool.QueryRow(ctx, `SELECT COALESCE(is_agent, false) FROM users WHERE lower(email) = lower($1)`, email).Scan(&isAgent)
	return err == nil && isAgent
}

// GetOrgSettingInt returns an integer org setting with a default fallback.
func (d *DB) GetOrgSettingInt(ctx context.Context, orgID int, key string, defaultVal int) int {
	val, err := d.GetOrgSetting(ctx, orgID, key)
	if err != nil || val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}

// CreateAgentNotification creates a notification marked as agent-actionable.
func (d *DB) CreateAgentNotification(ctx context.Context, orgID int, recipientEmail, title, body, link string) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO notifications (organization_id, recipient_id, title, body, link, agent_actionable)
		SELECT $1, u.id, $3, $4, $5, true
		FROM users u WHERE u.email = $2
	`, orgID, recipientEmail, title, nilIfEmpty(body), nilIfEmpty(link))
	return err
}

// ListReviewsByAuthorStatus returns reviews where the given user is the author and status matches.
func (d *DB) ListReviewsByAuthorStatus(ctx context.Context, orgID int, authorEmail, status string) ([]Review, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT r.id, r.organization_id, r.document_id, r.document_type, r.title, r.version, COALESCE(r.commit_hash, ''), COALESCE(r.sent_head, ''), COALESCE(r.merge_commit, ''), r.round,
			(SELECT email FROM users WHERE id = r.requested_by_id), COALESCE(r.message, ''), r.status, r.created_at, r.updated_at,
			(SELECT COUNT(*) FROM comments c WHERE c.review_id = r.id),
			(SELECT COUNT(*) FROM comments c WHERE c.review_id = r.id AND c.status = 'open')
		FROM reviews r
		WHERE r.organization_id = $1 AND r.status = $2
			AND r.requested_by_id = (SELECT id FROM users WHERE email = $3)
		ORDER BY r.updated_at DESC LIMIT 20
	`, orgID, status, authorEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []Review
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ID, &r.OrganizationID, &r.DocumentID, &r.DocumentType, &r.Title, &r.Version, &r.CommitHash, &r.SentHead, &r.MergeCommit, &r.Round, &r.RequestedBy, &r.Message, &r.Status, &r.CreatedAt, &r.UpdatedAt, &r.CommentCount, &r.OpenComments); err != nil {
			return nil, err
		}
		reviews = append(reviews, r)
	}
	return reviews, nil
}

// ListPendingAssignmentsForReviewer returns reviews where the given user has a pending assignment.
func (d *DB) ListPendingAssignmentsForReviewer(ctx context.Context, orgID int, reviewerEmail string) ([]Review, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT r.id, r.organization_id, r.document_id, r.document_type, r.title, r.version, COALESCE(r.commit_hash, ''), COALESCE(r.sent_head, ''), COALESCE(r.merge_commit, ''), r.round,
			(SELECT email FROM users WHERE id = r.requested_by_id), COALESCE(r.message, ''), r.status, r.created_at, r.updated_at,
			(SELECT COUNT(*) FROM comments c WHERE c.review_id = r.id),
			(SELECT COUNT(*) FROM comments c WHERE c.review_id = r.id AND c.status = 'open')
		FROM reviews r
		JOIN review_assignments ra ON ra.review_id = r.id AND ra.organization_id = r.organization_id
		WHERE r.organization_id = $1 AND r.status IN ('open', 'changes_requested')
			AND ra.reviewer_id = (SELECT id FROM users WHERE email = $2) AND ra.status = 'pending'
		ORDER BY r.updated_at DESC LIMIT 20
	`, orgID, reviewerEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []Review
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ID, &r.OrganizationID, &r.DocumentID, &r.DocumentType, &r.Title, &r.Version, &r.CommitHash, &r.SentHead, &r.MergeCommit, &r.Round, &r.RequestedBy, &r.Message, &r.Status, &r.CreatedAt, &r.UpdatedAt, &r.CommentCount, &r.OpenComments); err != nil {
			return nil, err
		}
		reviews = append(reviews, r)
	}
	return reviews, nil
}

func (d *DB) UpdateReviewStatus(ctx context.Context, orgID int, id int, status string) error {
	_, err := d.pool.Exec(ctx, `UPDATE reviews SET status = $2, updated_at = now() WHERE id = $1 AND organization_id = $3`, id, status, orgID)
	return err
}

// UpdateReviewSentHead updates the sent_head snapshot, increments the round, and resets status for a resubmitted review.
func (d *DB) UpdateReviewSentHead(ctx context.Context, orgID int, id int, sentHead string) error {
	// Mark all existing open comments on this review as outdated (new round)
	_, _ = d.pool.Exec(ctx, `UPDATE comments SET is_outdated = true WHERE review_id = $1 AND organization_id = $2 AND is_outdated = false`, id, orgID)
	_, err := d.pool.Exec(ctx, `UPDATE reviews SET sent_head = $2, status = 'open', round = round + 1, updated_at = now() WHERE id = $1 AND organization_id = $3`, id, sentHead, orgID)
	return err
}

func (d *DB) SetMergeCommit(ctx context.Context, orgID int, id int, mergeCommit string) error {
	_, err := d.pool.Exec(ctx, `UPDATE reviews SET merge_commit = $2, updated_at = now() WHERE id = $1 AND organization_id = $3`, id, mergeCommit, orgID)
	return err
}

// GetLastApprovedReview returns the most recent approved review for a document.
// Returns nil, nil if no approved review exists.
func (d *DB) GetLastApprovedReview(ctx context.Context, orgID int, documentID string) (*Review, error) {
	var r Review
	err := d.pool.QueryRow(ctx, `
		SELECT r.id, r.organization_id, r.document_id, r.document_type, r.title, r.version, COALESCE(r.commit_hash, ''), COALESCE(r.sent_head, ''), COALESCE(r.merge_commit, ''), r.round,
			(SELECT email FROM users WHERE id = r.requested_by_id), COALESCE(r.message, ''), r.status, r.created_at, r.updated_at
		FROM reviews r WHERE r.organization_id = $1 AND r.document_id = $2 AND r.status IN ('approved', 'merged')
		ORDER BY r.updated_at DESC LIMIT 1
	`, orgID, documentID).Scan(&r.ID, &r.OrganizationID, &r.DocumentID, &r.DocumentType, &r.Title, &r.Version, &r.CommitHash, &r.SentHead, &r.MergeCommit, &r.Round, &r.RequestedBy, &r.Message, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // no approved review found
		}
		return nil, fmt.Errorf("querying last approved review: %w", err)
	}
	return &r, nil
}

// GetOpenReviewForDocument returns any open or changes_requested review for a document.
// Returns nil, nil if no such review exists.
func (d *DB) GetOpenReviewForDocument(ctx context.Context, orgID int, documentID string) (*Review, error) {
	var r Review
	err := d.pool.QueryRow(ctx, `
		SELECT r.id, r.organization_id, r.document_id, r.document_type, r.title, r.version, COALESCE(r.commit_hash, ''), COALESCE(r.sent_head, ''), COALESCE(r.merge_commit, ''), r.round,
			(SELECT email FROM users WHERE id = r.requested_by_id), COALESCE(r.message, ''), r.status, r.created_at, r.updated_at
		FROM reviews r WHERE r.organization_id = $1 AND r.document_id = $2 AND r.status IN ('open', 'changes_requested')
		ORDER BY r.updated_at DESC LIMIT 1
	`, orgID, documentID).Scan(&r.ID, &r.OrganizationID, &r.DocumentID, &r.DocumentType, &r.Title, &r.Version, &r.CommitHash, &r.SentHead, &r.MergeCommit, &r.Round, &r.RequestedBy, &r.Message, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // no open review found
		}
		return nil, fmt.Errorf("querying open review: %w", err)
	}
	return &r, nil
}

// --- Comments ---

type Comment struct {
	ID             int        `json:"id"`
	OrganizationID int        `json:"organization_id"`
	ReviewID       *int       `json:"review_id,omitempty"`
	DocumentID     string     `json:"document_id"`
	Author         string     `json:"author"`
	Body           string     `json:"body"`
	Section        string     `json:"section,omitempty"`
	ParagraphIndex *int       `json:"paragraph_index,omitempty"`
	ParagraphHash  string     `json:"paragraph_hash,omitempty"`
	Quote          string     `json:"quote,omitempty"`
	ParentID       *int       `json:"parent_id,omitempty"`
	Status         string     `json:"status"`
	ResolvedBy           string  `json:"resolved_by,omitempty"`
	ResolvedAt           *Epoch  `json:"resolved_at,omitempty"`
	SuggestionBody       *string `json:"suggestion_body,omitempty"`
	SuggestionStatus     *string `json:"suggestion_status,omitempty"`
	SuggestionResolvedBy string  `json:"suggestion_resolved_by,omitempty"`
	SuggestionResolvedAt *Epoch  `json:"suggestion_resolved_at,omitempty"`
	IsOutdated           bool    `json:"is_outdated"`
	CreatedAt            Epoch   `json:"created_at"`
}

func (d *DB) AddComment(ctx context.Context, orgID int, c *Comment) error {
	c.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO comments (organization_id, review_id, document_id, author, author_user_id, body, section, paragraph_index, paragraph_hash, quote, parent_id, status, suggestion_body, suggestion_status)
		VALUES ($1, $2, $3, $4, (SELECT id FROM users WHERE email = $4), $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at
	`, orgID, c.ReviewID, c.DocumentID, c.Author, c.Body, c.Section, c.ParagraphIndex, c.ParagraphHash, c.Quote, c.ParentID, "open", c.SuggestionBody, c.SuggestionStatus,
	).Scan(&c.ID, &c.CreatedAt)
}

func (d *DB) GetCommentDocumentID(ctx context.Context, orgID int, id int) (string, error) {
	var docID string
	err := d.pool.QueryRow(ctx, `SELECT document_id FROM comments WHERE id = $1 AND organization_id = $2`, id, orgID).Scan(&docID)
	return docID, err
}

func (d *DB) ResolveComment(ctx context.Context, orgID int, id int, resolvedBy string) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE comments SET status = 'resolved', resolved_by_id = (SELECT id FROM users WHERE email = $2), resolved_at = now() WHERE id = $1 AND organization_id = $3
	`, id, resolvedBy, orgID)
	return err
}

func (d *DB) GetComment(ctx context.Context, orgID int, id int) (*Comment, error) {
	var c Comment
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, review_id, document_id, author, body, COALESCE(section, ''), paragraph_index, COALESCE(paragraph_hash, ''),
			COALESCE(quote, ''), parent_id, status, COALESCE((SELECT email FROM users WHERE id = comments.resolved_by_id), ''), resolved_at,
			suggestion_body, suggestion_status, COALESCE((SELECT email FROM users WHERE id = comments.suggestion_resolved_by_id), ''), suggestion_resolved_at, is_outdated, created_at
		FROM comments WHERE id = $1 AND organization_id = $2
	`, id, orgID).Scan(&c.ID, &c.OrganizationID, &c.ReviewID, &c.DocumentID, &c.Author, &c.Body, &c.Section, &c.ParagraphIndex, &c.ParagraphHash,
		&c.Quote, &c.ParentID, &c.Status, &c.ResolvedBy, &c.ResolvedAt, &c.SuggestionBody, &c.SuggestionStatus, &c.SuggestionResolvedBy, &c.SuggestionResolvedAt, &c.IsOutdated, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (d *DB) AcceptSuggestion(ctx context.Context, orgID int, commentID int, resolvedByEmail string) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE comments SET suggestion_status = 'accepted',
			suggestion_resolved_by_id = (SELECT id FROM users WHERE email = $2),
			suggestion_resolved_at = now(),
			status = 'resolved',
			resolved_by_id = (SELECT id FROM users WHERE email = $2),
			resolved_at = now()
		WHERE id = $1 AND organization_id = $3 AND suggestion_body IS NOT NULL AND suggestion_status = 'pending'
	`, commentID, resolvedByEmail, orgID)
	return err
}

func (d *DB) RejectSuggestion(ctx context.Context, orgID int, commentID int, resolvedByEmail string) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE comments SET suggestion_status = 'rejected',
			suggestion_resolved_by_id = (SELECT id FROM users WHERE email = $2),
			suggestion_resolved_at = now()
		WHERE id = $1 AND organization_id = $3 AND suggestion_body IS NOT NULL AND suggestion_status = 'pending'
	`, commentID, resolvedByEmail, orgID)
	return err
}

// CountPendingSuggestionsForReview returns the number of pending suggestion comments
// attached to a review. Used by the document banner to tell the author how many
// inline suggestions still need to be addressed in the current round.
func (d *DB) CountPendingSuggestionsForReview(ctx context.Context, orgID int, reviewID int) (int, error) {
	var n int
	err := d.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM comments
		WHERE organization_id = $1 AND review_id = $2
		  AND suggestion_body IS NOT NULL AND suggestion_status = 'pending'
		  AND COALESCE(is_outdated, false) = false
	`, orgID, reviewID).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// CommentsForDocument returns comments for a document, optionally filtered by review_id.
func (d *DB) CommentsForDocument(ctx context.Context, orgID int, documentID string, reviewID ...int) ([]Comment, error) {
	query := `
		SELECT id, organization_id, review_id, document_id, author, body, COALESCE(section, ''), paragraph_index, COALESCE(paragraph_hash, ''),
			COALESCE(quote, ''), parent_id, status, COALESCE((SELECT email FROM users WHERE id = comments.resolved_by_id), ''), resolved_at,
			suggestion_body, suggestion_status, COALESCE((SELECT email FROM users WHERE id = comments.suggestion_resolved_by_id), ''), suggestion_resolved_at, is_outdated, created_at
		FROM comments WHERE organization_id = $1 AND document_id = $2`
	args := []interface{}{orgID, documentID}
	if len(reviewID) > 0 && reviewID[0] > 0 {
		// Filter to specific review
		query += ` AND review_id = $3`
		args = append(args, reviewID[0])
	} else {
		// No review specified — exclude review-scoped comments, show only document-level
		query += ` AND review_id IS NULL`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.OrganizationID, &c.ReviewID, &c.DocumentID, &c.Author, &c.Body, &c.Section, &c.ParagraphIndex, &c.ParagraphHash, &c.Quote, &c.ParentID, &c.Status, &c.ResolvedBy, &c.ResolvedAt, &c.SuggestionBody, &c.SuggestionStatus, &c.SuggestionResolvedBy, &c.SuggestionResolvedAt, &c.IsOutdated, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, nil
}

func (d *DB) OpenCommentCounts(ctx context.Context, orgID int) (map[string]int, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT document_id, COUNT(*) FROM comments WHERE organization_id = $1 AND status = 'open' GROUP BY document_id
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var docID string
		var count int
		if err := rows.Scan(&docID, &count); err != nil {
			return nil, err
		}
		counts[docID] = count
	}
	return counts, nil
}

// AllOpenComments returns all open comments across all documents.
func (d *DB) AllOpenComments(ctx context.Context, orgID int) ([]Comment, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, review_id, document_id, author, body, COALESCE(section, ''), paragraph_index, COALESCE(paragraph_hash, ''),
			COALESCE(quote, ''), parent_id, status, COALESCE((SELECT email FROM users WHERE id = comments.resolved_by_id), ''), resolved_at,
			suggestion_body, suggestion_status, COALESCE((SELECT email FROM users WHERE id = comments.suggestion_resolved_by_id), ''), suggestion_resolved_at, is_outdated, created_at
		FROM comments WHERE organization_id = $1 AND status = 'open'
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.OrganizationID, &c.ReviewID, &c.DocumentID, &c.Author, &c.Body, &c.Section, &c.ParagraphIndex, &c.ParagraphHash, &c.Quote, &c.ParentID, &c.Status, &c.ResolvedBy, &c.ResolvedAt, &c.SuggestionBody, &c.SuggestionStatus, &c.SuggestionResolvedBy, &c.SuggestionResolvedAt, &c.IsOutdated, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, nil
}

// --- Approvals ---

type Approval struct {
	ID             int       `json:"id"`
	OrganizationID int       `json:"organization_id"`
	ReviewID       *int      `json:"review_id,omitempty"`
	DocumentID     string    `json:"document_id"`
	Version        string    `json:"version"`
	Round          int       `json:"round"`
	Decision       string    `json:"decision"` // approved, changes_requested
	ApprovedBy     string    `json:"approved_by"`
	Comment        string `json:"comment,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
}

func (d *DB) AddApproval(ctx context.Context, orgID int, a *Approval) error {
	a.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO approvals (organization_id, review_id, document_id, version, round, decision, approved_by, approved_by_user_id, comment)
		VALUES ($1, $2, $3, $4, $5, $6, $7, (SELECT id FROM users WHERE email = $7), $8)
		RETURNING id, created_at
	`, orgID, a.ReviewID, a.DocumentID, a.Version, a.Round, a.Decision, a.ApprovedBy, a.Comment,
	).Scan(&a.ID, &a.CreatedAt)
}

func (d *DB) ApprovalsForDocument(ctx context.Context, orgID int, documentID string) ([]Approval, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, review_id, document_id, version, round, decision, approved_by, COALESCE(comment, ''), created_at
		FROM approvals WHERE organization_id = $1 AND document_id = $2
		ORDER BY created_at DESC
	`, orgID, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []Approval
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.ReviewID, &a.DocumentID, &a.Version, &a.Round, &a.Decision, &a.ApprovedBy, &a.Comment, &a.CreatedAt); err != nil {
			return nil, err
		}
		approvals = append(approvals, a)
	}
	return approvals, nil
}

// --- Activity ---

type Activity struct {
	ID             int       `json:"id"`
	OrganizationID int       `json:"organization_id"`
	DocumentID     string    `json:"document_id,omitempty"`
	ReviewID       *int      `json:"review_id,omitempty"`
	Actor          string    `json:"actor"`
	Action         string    `json:"action"`
	Detail         string `json:"detail,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
}

func (d *DB) LogActivity(ctx context.Context, orgID int, a *Activity) error {
	a.OrganizationID = orgID
	_, err := d.pool.Exec(ctx, `
		INSERT INTO activity (organization_id, document_id, review_id, actor, actor_user_id, action, detail)
		VALUES ($1, $2, $3, $4, (SELECT id FROM users WHERE email = $4), $5, $6)
	`, orgID, a.DocumentID, a.ReviewID, a.Actor, a.Action, a.Detail)
	return err
}

func (d *DB) RecentActivity(ctx context.Context, orgID int, limit int) ([]Activity, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, COALESCE(document_id, ''), review_id, actor, action, COALESCE(detail, ''), created_at
		FROM activity WHERE organization_id = $1 ORDER BY created_at DESC LIMIT $2
	`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.DocumentID, &a.ReviewID, &a.Actor, &a.Action, &a.Detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}
	return activities, nil
}

func (d *DB) ActivityForReview(ctx context.Context, orgID int, reviewID int, limit int) ([]Activity, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, COALESCE(document_id, ''), review_id, actor, action, COALESCE(detail, ''), created_at
		FROM activity WHERE organization_id = $1 AND review_id = $2 ORDER BY created_at ASC LIMIT $3
	`, orgID, reviewID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.DocumentID, &a.ReviewID, &a.Actor, &a.Action, &a.Detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}
	return activities, nil
}

func (d *DB) CommentsForReview(ctx context.Context, orgID int, reviewID int) ([]Comment, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, review_id, document_id, author, body, COALESCE(section, ''), paragraph_index, COALESCE(paragraph_hash, ''),
			COALESCE(quote, ''), parent_id, status, COALESCE((SELECT email FROM users WHERE id = comments.resolved_by_id), ''), resolved_at,
			suggestion_body, suggestion_status, COALESCE((SELECT email FROM users WHERE id = comments.suggestion_resolved_by_id), ''), suggestion_resolved_at, is_outdated, created_at
		FROM comments WHERE organization_id = $1 AND review_id = $2 ORDER BY created_at ASC
	`, orgID, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.OrganizationID, &c.ReviewID, &c.DocumentID, &c.Author, &c.Body, &c.Section, &c.ParagraphIndex, &c.ParagraphHash, &c.Quote, &c.ParentID, &c.Status, &c.ResolvedBy, &c.ResolvedAt, &c.SuggestionBody, &c.SuggestionStatus, &c.SuggestionResolvedBy, &c.SuggestionResolvedAt, &c.IsOutdated, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, nil
}

func (d *DB) ApprovalsForReview(ctx context.Context, orgID int, reviewID int) ([]Approval, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, review_id, document_id, version, round, decision, approved_by, COALESCE(comment, ''), created_at
		FROM approvals WHERE organization_id = $1 AND review_id = $2 ORDER BY created_at ASC
	`, orgID, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []Approval
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.ReviewID, &a.DocumentID, &a.Version, &a.Round, &a.Decision, &a.ApprovedBy, &a.Comment, &a.CreatedAt); err != nil {
			return nil, err
		}
		approvals = append(approvals, a)
	}
	return approvals, nil
}

func (d *DB) ActivityForDocument(ctx context.Context, orgID int, documentID string, limit int) ([]Activity, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, document_id, review_id, actor, action, COALESCE(detail, ''), created_at
		FROM activity WHERE organization_id = $1 AND document_id = $2 ORDER BY created_at DESC LIMIT $3
	`, orgID, documentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.DocumentID, &a.ReviewID, &a.Actor, &a.Action, &a.Detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}
	return activities, nil
}
