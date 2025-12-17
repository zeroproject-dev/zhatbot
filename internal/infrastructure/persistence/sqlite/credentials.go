package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"zhatBot/internal/domain"
)

type CredentialStore struct {
	db *sql.DB
}

func NewCredentialStore(dbPath string) (*CredentialStore, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("sqlite: empty db path")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("sqlite: creating dir: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}

	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return &CredentialStore{db: db}, nil
}

func migrate(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS credentials (
	platform TEXT NOT NULL,
	role TEXT NOT NULL,
	access_token TEXT NOT NULL,
	refresh_token TEXT,
	expires_at TIMESTAMP,
	updated_at TIMESTAMP NOT NULL,
	metadata TEXT,
	PRIMARY KEY (platform, role)
);`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("sqlite: migrate credentials: %w", err)
	}

	if _, err := db.Exec(`ALTER TABLE credentials ADD COLUMN metadata TEXT;`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("sqlite: add metadata column: %w", err)
		}
	}

	return nil
}

func (s *CredentialStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *CredentialStore) Get(ctx context.Context, platform domain.Platform, role string) (*domain.Credential, error) {
	const query = `
SELECT access_token, refresh_token, expires_at, updated_at, metadata
FROM credentials
WHERE platform = ? AND role = ?
LIMIT 1;
`

	row := s.db.QueryRowContext(ctx, query, string(platform), role)

	var accessToken, refreshToken, metadata sql.NullString
	var expiresAt, updatedAt sql.NullTime

	if err := row.Scan(&accessToken, &refreshToken, &expiresAt, &updatedAt, &metadata); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: get credential: %w", err)
	}

	return &domain.Credential{
		Platform:     platform,
		Role:         role,
		AccessToken:  accessToken.String,
		RefreshToken: refreshToken.String,
		ExpiresAt:    expiresAt.Time,
		UpdatedAt:    updatedAt.Time,
		Metadata:     decodeMetadata(metadata.String),
	}, nil
}

func (s *CredentialStore) Save(ctx context.Context, cred *domain.Credential) error {
	if cred == nil {
		return fmt.Errorf("sqlite: credential nil")
	}

	now := time.Now().UTC()
	if cred.UpdatedAt.IsZero() {
		cred.UpdatedAt = now
	}

	const stmt = `
INSERT INTO credentials (platform, role, access_token, refresh_token, expires_at, updated_at, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(platform, role) DO UPDATE SET
	access_token=excluded.access_token,
	refresh_token=excluded.refresh_token,
	expires_at=excluded.expires_at,
	updated_at=excluded.updated_at,
	metadata=excluded.metadata;
`

	_, err := s.db.ExecContext(
		ctx,
		stmt,
		string(cred.Platform),
		cred.Role,
		cred.AccessToken,
		cred.RefreshToken,
		nullTime(cred.ExpiresAt),
		cred.UpdatedAt,
		encodeMetadata(cred.Metadata),
	)
	if err != nil {
		return fmt.Errorf("sqlite: save credential: %w", err)
	}

	return nil
}

func (s *CredentialStore) List(ctx context.Context) ([]*domain.Credential, error) {
	const query = `
SELECT platform, role, access_token, refresh_token, expires_at, updated_at, metadata
FROM credentials;
`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list credentials: %w", err)
	}
	defer rows.Close()

	var out []*domain.Credential
	for rows.Next() {
		var platform string
		var role string
		var accessToken, refreshToken, metadata sql.NullString
		var expiresAt, updatedAt sql.NullTime
		if err := rows.Scan(&platform, &role, &accessToken, &refreshToken, &expiresAt, &updatedAt, &metadata); err != nil {
			return nil, fmt.Errorf("sqlite: scan credential: %w", err)
		}

		out = append(out, &domain.Credential{
			Platform:     domain.Platform(platform),
			Role:         role,
			AccessToken:  accessToken.String,
			RefreshToken: refreshToken.String,
			ExpiresAt:    expiresAt.Time,
			UpdatedAt:    updatedAt.Time,
			Metadata:     decodeMetadata(metadata.String),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list rows error: %w", err)
	}

	return out, nil
}

func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func encodeMetadata(data map[string]string) interface{} {
	if len(data) == 0 {
		return nil
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	return string(encoded)
}

func decodeMetadata(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var metadata map[string]string
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil
	}
	return metadata
}

var _ domain.CredentialRepository = (*CredentialStore)(nil)
