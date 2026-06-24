package db

import (
	"context"
	"fmt"
)

// Setting is a known setting from the settings registry.
type Setting struct {
	Key          string  `json:"key"`
	Description  string  `json:"description"`
	Category     string  `json:"category"`
	DefaultValue *string `json:"default_value,omitempty"`
	Sensitive    bool    `json:"sensitive"`
}

// OrgSetting is a per-org setting value.
type OrgSetting struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// ListSettings returns all known settings.
func (d *DB) ListSettings(ctx context.Context) ([]Setting, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT key, description, category, default_value, sensitive
		FROM settings ORDER BY category, key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []Setting
	for rows.Next() {
		var s Setting
		if err := rows.Scan(&s.Key, &s.Description, &s.Category, &s.DefaultValue, &s.Sensitive); err != nil {
			return nil, err
		}
		settings = append(settings, s)
	}
	return settings, nil
}

// GetOrgSetting returns a single setting value for an org. Falls back to default. Decrypts if sensitive.
func (d *DB) GetOrgSetting(ctx context.Context, orgID int, key string) (string, error) {
	var value string
	var sensitive bool
	err := d.pool.QueryRow(ctx, `
		SELECT COALESCE(os.value, s.default_value, ''), s.sensitive
		FROM settings s
		LEFT JOIN organization_settings os ON os.setting_key = s.key AND os.organization_id = $1
		WHERE s.key = $2
	`, orgID, key).Scan(&value, &sensitive)
	if err != nil {
		return "", err
	}
	if sensitive && d.encryptionKey != "" && value != "" {
		dec, decErr := DecryptSecret(value, d.encryptionKey)
		if decErr != nil {
			return "", fmt.Errorf("decrypting setting %s: %w", key, decErr)
		}
		return dec, nil
	}
	return value, nil
}

// GetOrgSettings returns all settings for an org with values (or defaults).
// Sensitive settings are decrypted before return — the admin-only caller needs
// the cleartext to display in the settings UI (masked client-side with reveal).
func (d *DB) GetOrgSettings(ctx context.Context, orgID int) ([]OrgSetting, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT s.key, COALESCE(os.value, s.default_value, ''), s.description, s.category, s.sensitive
		FROM settings s
		LEFT JOIN organization_settings os ON os.setting_key = s.key AND os.organization_id = $1
		ORDER BY s.category, s.key
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []OrgSetting
	for rows.Next() {
		var s OrgSetting
		var sensitive bool
		if err := rows.Scan(&s.Key, &s.Value, &s.Description, &s.Category, &sensitive); err != nil {
			return nil, err
		}
		if sensitive && d.encryptionKey != "" && s.Value != "" {
			if dec, decErr := DecryptSecret(s.Value, d.encryptionKey); decErr == nil {
				s.Value = dec
			} else {
				// Decrypt failure means the stored value is not a valid ciphertext
				// (e.g. legacy plaintext from before encryption was enabled). Return
				// empty rather than leak the encrypted blob to the UI.
				s.Value = ""
			}
		}
		settings = append(settings, s)
	}
	return settings, nil
}

// SetOrgSetting sets a setting value for an org (upsert). Encrypts if setting is sensitive.
func (d *DB) SetOrgSetting(ctx context.Context, orgID int, key, value string) error {
	// Check if this setting is sensitive
	var sensitive bool
	_ = d.pool.QueryRow(ctx, `SELECT sensitive FROM settings WHERE key = $1`, key).Scan(&sensitive)
	storeValue := value
	if sensitive && d.encryptionKey != "" {
		enc, err := EncryptSecret(value, d.encryptionKey)
		if err != nil {
			return fmt.Errorf("encrypt setting %s: %w", key, err)
		}
		storeValue = enc
	}
	_, err := d.pool.Exec(ctx, `
		INSERT INTO organization_settings (organization_id, setting_key, value)
		VALUES ($1, $2, $3)
		ON CONFLICT (organization_id, setting_key) DO UPDATE SET value = EXCLUDED.value
	`, orgID, key, storeValue)
	return err
}

// DeleteOrgSetting removes a setting value for an org (reverts to default).
func (d *DB) DeleteOrgSetting(ctx context.Context, orgID int, key string) error {
	_, err := d.pool.Exec(ctx, `
		DELETE FROM organization_settings WHERE organization_id = $1 AND setting_key = $2
	`, orgID, key)
	return err
}
