package database

import (
	"database/sql"
	"encoding/json"
	"time"
)

// KeyserverPublish records one successful publish of the project public key to a
// keyserver (PART 11 → GPG Keypair Management → keyservers_published).
type KeyserverPublish struct {
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}

// SecurityKeypair is the on-disk project PGP keypair's DB-tracked metadata
// (AI.md 14189-14198). The key material itself is never stored here — only the
// fingerprint, lifecycle timestamps, keyserver publish state, and revoked flag.
type SecurityKeypair struct {
	// Fingerprint is the primary key's OpenPGP fingerprint (uppercase hex).
	Fingerprint string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	// LastRotatedAt is set when the keypair was most recently rotated (nil if never).
	LastRotatedAt *time.Time
	// KeyserversPublished lists keyservers the public key has been pushed to.
	KeyserversPublished []KeyserverPublish
	// Revoked is set when the operator deleted the keypair; the fingerprint is
	// retained for audit history even after the key files are gone.
	Revoked bool
}

// GetSecurityKeypair returns the single keypair-metadata row, or (nil, nil) when
// no keypair has been generated yet.
func (s *SQLiteDB) GetSecurityKeypair() (*SecurityKeypair, error) {
	kp := &SecurityKeypair{}
	var expiresAt, lastRotatedAt sql.NullTime
	var published string
	var revoked int
	ctx, cancel := dbCtx(dbReadTimeout)
	defer cancel()
	err := s.db.QueryRowContext(ctx,
		`SELECT fingerprint, created_at, expires_at, last_rotated_at,
			keyservers_published, revoked
		 FROM security_keypair WHERE id = 1`,
	).Scan(&kp.Fingerprint, &kp.CreatedAt, &expiresAt, &lastRotatedAt,
		&published, &revoked)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		kp.ExpiresAt = expiresAt.Time
	}
	if lastRotatedAt.Valid {
		kp.LastRotatedAt = &lastRotatedAt.Time
	}
	if published != "" {
		if err := json.Unmarshal([]byte(published), &kp.KeyserversPublished); err != nil {
			return nil, err
		}
	}
	kp.Revoked = revoked != 0
	return kp, nil
}

// UpsertSecurityKeypair inserts or replaces the single keypair-metadata row.
func (s *SQLiteDB) UpsertSecurityKeypair(kp *SecurityKeypair) error {
	published := "[]"
	if len(kp.KeyserversPublished) > 0 {
		b, err := json.Marshal(kp.KeyserversPublished)
		if err != nil {
			return err
		}
		published = string(b)
	}
	var expiresAt, lastRotatedAt any
	if !kp.ExpiresAt.IsZero() {
		expiresAt = kp.ExpiresAt
	}
	if kp.LastRotatedAt != nil {
		lastRotatedAt = *kp.LastRotatedAt
	}
	revoked := 0
	if kp.Revoked {
		revoked = 1
	}
	ctx, cancel := dbCtx(dbWriteTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO security_keypair
			(id, fingerprint, created_at, expires_at, last_rotated_at,
			 keyservers_published, revoked)
			VALUES (1, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			fingerprint = excluded.fingerprint,
			created_at = excluded.created_at,
			expires_at = excluded.expires_at,
			last_rotated_at = excluded.last_rotated_at,
			keyservers_published = excluded.keyservers_published,
			revoked = excluded.revoked`,
		kp.Fingerprint, kp.CreatedAt, expiresAt, lastRotatedAt, published, revoked,
	)
	return err
}
