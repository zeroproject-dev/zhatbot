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

	const customCommandsTable = `
CREATE TABLE IF NOT EXISTS custom_commands (
	name TEXT PRIMARY KEY,
	response TEXT NOT NULL,
	aliases TEXT,
	platforms TEXT,
	updated_at TIMESTAMP NOT NULL
);`

	if _, err := db.Exec(customCommandsTable); err != nil {
		return fmt.Errorf("sqlite: migrate custom_commands: %w", err)
	}

	const settingsTable = `
CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT,
	updated_at TIMESTAMP NOT NULL
);`

	if _, err := db.Exec(settingsTable); err != nil {
		return fmt.Errorf("sqlite: migrate settings: %w", err)
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

// Custom command storage

func (s *CredentialStore) UpsertCustomCommand(ctx context.Context, cmd *domain.CustomCommand) error {
	if cmd == nil {
		return fmt.Errorf("sqlite: custom command nil")
	}

	now := time.Now().UTC()
	if cmd.UpdatedAt.IsZero() {
		cmd.UpdatedAt = now
	}

	const stmt = `
INSERT INTO custom_commands (name, response, aliases, platforms, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
	response=excluded.response,
	aliases=excluded.aliases,
	platforms=excluded.platforms,
	updated_at=excluded.updated_at;
`

	_, err := s.db.ExecContext(
		ctx,
		stmt,
		cmd.Name,
		cmd.Response,
		encodeStringSlice(cmd.Aliases),
		encodePlatforms(cmd.Platforms),
		cmd.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("sqlite: upsert custom command: %w", err)
	}

	return nil
}

func (s *CredentialStore) GetCustomCommand(ctx context.Context, name string) (*domain.CustomCommand, error) {
	const query = `
SELECT name, response, aliases, platforms, updated_at
FROM custom_commands
WHERE LOWER(name) = LOWER(?)
LIMIT 1;
`

	row := s.db.QueryRowContext(ctx, query, name)

	var record domain.CustomCommand
	var aliasesRaw, platformsRaw sql.NullString
	var updatedAt sql.NullTime

	if err := row.Scan(&record.Name, &record.Response, &aliasesRaw, &platformsRaw, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: get custom command: %w", err)
	}

	record.Aliases = decodeStringSlice(aliasesRaw.String)
	record.Platforms = decodePlatforms(platformsRaw.String)
	record.UpdatedAt = updatedAt.Time

	return &record, nil
}

func (s *CredentialStore) ListCustomCommands(ctx context.Context) ([]*domain.CustomCommand, error) {
	const query = `
SELECT name, response, aliases, platforms, updated_at
FROM custom_commands;
`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list custom commands: %w", err)
	}
	defer rows.Close()

	var cmds []*domain.CustomCommand
	for rows.Next() {
		var record domain.CustomCommand
		var aliasesRaw, platformsRaw sql.NullString
		var updatedAt sql.NullTime

		if err := rows.Scan(&record.Name, &record.Response, &aliasesRaw, &platformsRaw, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan custom command: %w", err)
		}

		record.Aliases = decodeStringSlice(aliasesRaw.String)
		record.Platforms = decodePlatforms(platformsRaw.String)
		record.UpdatedAt = updatedAt.Time

		cmds = append(cmds, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list custom command rows: %w", err)
	}

	return cmds, nil
}

func encodeStringSlice(values []string) interface{} {
	clean := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			clean = append(clean, v)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	b, err := json.Marshal(clean)
	if err != nil {
		return nil
	}
	return string(b)
}

func decodeStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func encodePlatforms(values []domain.Platform) interface{} {
	if len(values) == 0 {
		return nil
	}
	stringsVals := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		stringsVals = append(stringsVals, string(v))
	}
	if len(stringsVals) == 0 {
		return nil
	}
	b, err := json.Marshal(stringsVals)
	if err != nil {
		return nil
	}
	return string(b)
}

func decodePlatforms(raw string) []domain.Platform {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	out := make([]domain.Platform, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, domain.Platform(v))
	}
	return out
}

var _ domain.CustomCommandRepository = (*CredentialStore)(nil)

func (s *CredentialStore) DeleteCustomCommand(ctx context.Context, name string) error {
	const stmt = `DELETE FROM custom_commands WHERE LOWER(name) = LOWER(?);`
	if _, err := s.db.ExecContext(ctx, stmt, name); err != nil {
		return fmt.Errorf("sqlite: delete custom command: %w", err)
	}
	return nil
}

// ----- TTS Settings -----

const ttsVoiceKey = "tts_voice"

func (s *CredentialStore) SetTTSVoice(ctx context.Context, voice string) error {
	return s.setSetting(ctx, ttsVoiceKey, voice)
}

func (s *CredentialStore) GetTTSVoice(ctx context.Context) (string, error) {
	return s.getSetting(ctx, ttsVoiceKey)
}

func (s *CredentialStore) setSetting(ctx context.Context, key, value string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("sqlite: empty setting key")
	}

	now := time.Now().UTC()
	const stmt = `
INSERT INTO settings (key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
	value=excluded.value,
	updated_at=excluded.updated_at;
`

	if _, err := s.db.ExecContext(ctx, stmt, key, value, now); err != nil {
		return fmt.Errorf("sqlite: set setting: %w", err)
	}

	return nil
}

func (s *CredentialStore) getSetting(ctx context.Context, key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("sqlite: empty setting key")
	}

	const query = `SELECT value FROM settings WHERE key = ? LIMIT 1;`
	row := s.db.QueryRowContext(ctx, query, key)

	var value sql.NullString
	if err := row.Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("sqlite: get setting: %w", err)
	}

	return value.String, nil
}

var _ domain.TTSSettingsRepository = (*CredentialStore)(nil)
