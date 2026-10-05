// P4-2 WebAuthn 通行密钥存储（规格书 §7 P4-2 / §11.1 admin_credentials 表）。
package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// StoredCredentialRow 单把通行密钥。
type StoredCredentialRow struct {
	ID              int64
	CredentialID    []byte
	PublicKey       []byte
	AttestationType string
	SignCount       uint32
	Transports      string // JSON 数组字符串
	CreatedAt       time.Time
}

// PutAdminCredential 写入（credential_id 唯一，重复忽略更新）。
func (db *DB) PutAdminCredential(ctx context.Context, credentialID, publicKey []byte, attestationType string, signCount uint32, transports []string) error {
	tj, _ := json.Marshal(transports)
	_, err := db.ExecContext(ctx,
		`INSERT INTO admin_credentials (credential_id, public_key, attestation_type, sign_count, transports, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(credential_id) DO UPDATE SET public_key = excluded.public_key,
		   attestation_type = excluded.attestation_type, sign_count = excluded.sign_count,
		   transports = excluded.transports`,
		credentialID, publicKey, attestationType, signCount, string(tj), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("写通行密钥: %w", err)
	}
	return nil
}

// ListAdminCredentials 全部通行密钥（单管理员模型）。
func (db *DB) ListAdminCredentials(ctx context.Context) ([]StoredCredentialRow, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT id, credential_id, public_key, attestation_type, sign_count, transports, created_at FROM admin_credentials ORDER BY created_at")
	if err != nil {
		return nil, fmt.Errorf("查询通行密钥: %w", err)
	}
	defer rows.Close()
	out := []StoredCredentialRow{}
	for rows.Next() {
		var c StoredCredentialRow
		var tj, created string
		if err := rows.Scan(&c.ID, &c.CredentialID, &c.PublicKey, &c.AttestationType, &c.SignCount, &tj, &created); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateAdminSignCount 登录成功后回写签名计数（防克隆检测的关键）。
func (db *DB) UpdateAdminSignCount(ctx context.Context, id int64, count uint32) error {
	_, err := db.ExecContext(ctx, "UPDATE admin_credentials SET sign_count = ? WHERE id = ?", count, id)
	if err != nil {
		return fmt.Errorf("更新签名计数: %w", err)
	}
	return nil
}

// DeleteAdminCredential 删除通行密钥。
func (db *DB) DeleteAdminCredential(ctx context.Context, id int64) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM admin_credentials WHERE id = ?", id); err != nil {
		return fmt.Errorf("删通行密钥: %w", err)
	}
	return nil
}
