// P4-2 WebAuthn 端口适配：sqlite admin_credentials → httpapi.PasskeyStore。
package cli

import (
	"context"
	"encoding/json"

	"github.com/NoraStory/GithubHot/internal/infrastructure/persistence/sqlite"
	"github.com/NoraStory/GithubHot/internal/interfaces/httpapi"
)

// passkeyStore 适配器（单管理员模型，凭据行即全部状态）。
type passkeyStore struct{ db *sqlite.DB }

func (p passkeyStore) ListAdminCredentials(ctx context.Context) ([]httpapi.StoredAdminCredential, error) {
	rows, err := p.db.ListAdminCredentials(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.StoredAdminCredential, 0, len(rows))
	for _, r := range rows {
		var transports []string
		_ = json.Unmarshal([]byte(r.Transports), &transports)
		out = append(out, httpapi.StoredAdminCredential{
			DBID: r.ID, CredentialID: r.CredentialID, PublicKey: r.PublicKey,
			AttestationType: r.AttestationType, SignCount: r.SignCount,
			Transports: transports, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

func (p passkeyStore) PutAdminCredential(ctx context.Context, credentialID, publicKey []byte,
	attestationType string, signCount uint32, transports []string) error {
	return p.db.PutAdminCredential(ctx, credentialID, publicKey, attestationType, signCount, transports)
}

func (p passkeyStore) UpdateAdminSignCount(ctx context.Context, id int64, count uint32) error {
	return p.db.UpdateAdminSignCount(ctx, id, count)
}

func (p passkeyStore) DeleteAdminCredential(ctx context.Context, id int64) error {
	return p.db.DeleteAdminCredential(ctx, id)
}
