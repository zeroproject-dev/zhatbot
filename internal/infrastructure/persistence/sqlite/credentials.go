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
	permissions TEXT,
	updated_at TIMESTAMP NOT NULL
);`

	if _, err := db.Exec(customCommandsTable); err != nil {
		return fmt.Errorf("sqlite: migrate custom_commands: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE custom_commands ADD COLUMN permissions TEXT;`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("sqlite: add permissions column: %w", err)
		}
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

	const notificationsTable = `
CREATE TABLE IF NOT EXISTS notifications (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	type TEXT NOT NULL,
	platform TEXT,
	username TEXT,
	amount REAL,
	message TEXT,
	metadata TEXT,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at DESC);`

	if _, err := db.Exec(notificationsTable); err != nil {
		return fmt.Errorf("sqlite: migrate notifications: %w", err)
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

func (s *CredentialStore) Delete(ctx context.Context, platform domain.Platform, role string) error {
	if s.db == nil {
		return fmt.Errorf("sqlite: db no inicializada")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE platform = ? AND role = ?`, string(platform), role)
	if err != nil {
		return fmt.Errorf("sqlite: delete credential: %w", err)
	}
	return nil
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
INSERT INTO custom_commands (name, response, aliases, platforms, permissions, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
	response=excluded.response,
	aliases=excluded.aliases,
	platforms=excluded.platforms,
	permissions=excluded.permissions,
	updated_at=excluded.updated_at;
`

	_, err := s.db.ExecContext(
		ctx,
		stmt,
		cmd.Name,
		cmd.Response,
		encodeStringSlice(cmd.Aliases),
		encodePlatforms(cmd.Platforms),
		encodePermissions(cmd.Permissions),
		cmd.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("sqlite: upsert custom command: %w", err)
	}

	return nil
}

func (s *CredentialStore) GetCustomCommand(ctx context.Context, name string) (*domain.CustomCommand, error) {
	const query = `
SELECT name, response, aliases, platforms, permissions, updated_at
FROM custom_commands
WHERE LOWER(name) = LOWER(?)
LIMIT 1;
`

	row := s.db.QueryRowContext(ctx, query, name)

	var record domain.CustomCommand
	var aliasesRaw, platformsRaw, permissionsRaw sql.NullString
	var updatedAt sql.NullTime

	if err := row.Scan(&record.Name, &record.Response, &aliasesRaw, &platformsRaw, &permissionsRaw, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: get custom command: %w", err)
	}

	record.Aliases = decodeStringSlice(aliasesRaw.String)
	record.Platforms = decodePlatforms(platformsRaw.String)
	record.Permissions = decodePermissions(permissionsRaw.String)
	record.UpdatedAt = updatedAt.Time

	return &record, nil
}

func (s *CredentialStore) ListCustomCommands(ctx context.Context) ([]*domain.CustomCommand, error) {
	const query = `
SELECT name, response, aliases, platforms, permissions, updated_at
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
		var aliasesRaw, platformsRaw, permissionsRaw sql.NullString
		var updatedAt sql.NullTime

		if err := rows.Scan(&record.Name, &record.Response, &aliasesRaw, &platformsRaw, &permissionsRaw, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan custom command: %w", err)
		}

		record.Aliases = decodeStringSlice(aliasesRaw.String)
		record.Platforms = decodePlatforms(platformsRaw.String)
		record.Permissions = decodePermissions(permissionsRaw.String)
		record.UpdatedAt = updatedAt.Time

		cmds = append(cmds, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list custom command rows: %w", err)
	}

	return cmds, nil
}

// ----- Notifications -----

func (s *CredentialStore) SaveNotification(ctx context.Context, notification *domain.Notification) (*domain.Notification, error) {
	if notification == nil {
		return nil, fmt.Errorf("sqlite: notification nil")
	}

	now := time.Now().UTC()
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = now
	}

	const stmt = `
INSERT INTO notifications (type, platform, username, amount, message, metadata, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);
`

	res, err := s.db.ExecContext(
		ctx,
		stmt,
		string(notification.Type),
		string(notification.Platform),
		notification.Username,
		notification.Amount,
		notification.Message,
		encodeMetadata(notification.Metadata),
		notification.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: save notification: %w", err)
	}

	if id, err := res.LastInsertId(); err == nil {
		notification.ID = id
	}

	return notification, nil
}

func (s *CredentialStore) ListNotifications(ctx context.Context, limit int) ([]*domain.Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	const query = `
SELECT id, type, platform, username, amount, message, metadata, created_at
FROM notifications
ORDER BY created_at DESC
LIMIT ?;
`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list notifications: %w", err)
	}
	defer rows.Close()

	var out []*domain.Notification
	for rows.Next() {
		var (
			record                 domain.Notification
			notificationType, plat sql.NullString
			username, message      sql.NullString
			metadata               sql.NullString
			amount                 sql.NullFloat64
			createdAt              sql.NullTime
		)

		if err := rows.Scan(
			&record.ID,
			&notificationType,
			&plat,
			&username,
			&amount,
			&message,
			&metadata,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("sqlite: scan notification: %w", err)
		}

		record.Type = domain.NotificationType(notificationType.String)
		record.Platform = domain.Platform(plat.String)
		record.Username = username.String
		record.Amount = amount.Float64
		record.Message = message.String
		record.Metadata = decodeMetadata(metadata.String)
		record.CreatedAt = createdAt.Time

		out = append(out, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list notifications rows: %w", err)
	}

	return out, nil
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

func encodePermissions(values []domain.CommandAccessRole) interface{} {
	if len(values) == 0 {
		return nil
	}
	clean := make([]string, 0, len(values))
	for _, role := range values {
		val := strings.TrimSpace(string(role))
		if val == "" {
			continue
		}
		clean = append(clean, val)
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

func decodePermissions(raw string) []domain.CommandAccessRole {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var entries []string
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil
	}
	var out []domain.CommandAccessRole
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		out = append(out, domain.CommandAccessRole(entry))
	}
	return out
}

var _ domain.CustomCommandRepository = (*CredentialStore)(nil)
var _ domain.NotificationRepository = (*CredentialStore)(nil)

func (s *CredentialStore) DeleteCustomCommand(ctx context.Context, name string) error {
	const stmt = `DELETE FROM custom_commands WHERE LOWER(name) = LOWER(?);`
	if _, err := s.db.ExecContext(ctx, stmt, name); err != nil {
		return fmt.Errorf("sqlite: delete custom command: %w", err)
	}
	return nil
}

// ----- TTS Settings -----

const ttsVoiceKey = "tts_voice"
const ttsEnabledKey = "tts_enabled"

func (s *CredentialStore) SetTTSVoice(ctx context.Context, voice string) error {
	return s.setSetting(ctx, ttsVoiceKey, voice)
}

func (s *CredentialStore) GetTTSVoice(ctx context.Context) (string, error) {
	return s.getSetting(ctx, ttsVoiceKey)
}

func (s *CredentialStore) SetTTSEnabled(ctx context.Context, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	return s.setSetting(ctx, ttsEnabledKey, value)
}

func (s *CredentialStore) GetTTSEnabled(ctx context.Context) (bool, error) {
	val, err := s.getSetting(ctx, ttsEnabledKey)
	if err != nil {
		return false, err
	}
	return strings.ToLower(strings.TrimSpace(val)) != "false", nil
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
