// P4-2 WebAuthn 通行密钥（规格书 §7 P4-2，管理端免密登录）。
//
// 端点（/api/v1/admin/passkey/*）：
//   begin-register / finish-register —— 已登录管理员注册新密钥（守卫组内）；
//   begin-login / finish-login       —— 免密登录入口（守卫组外，登录成功复用
//      gh_admin_session 会话通道并绑定 IP，同 P0-2 口径）；
//   credentials / credentials/{id}   —— 密钥列表与删除（守卫组内）。
//
// 纪律：
//   - SessionData 内存态：注册绑管理员会话 Cookie，登录用一次性 token（5 分钟 TTL）；
//   - finish-login 成功才建会话；签名计数回写（防克隆检测的关键）；
//   - WEBAUTHN_ONLY=1 时密码登录禁用；Argon2id 永远保留为后备（规格）。
package httpapi

import (
	"context"
	"errors"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// PasskeyStore 通行密钥存储端口（sqlite admin_credentials 实现）。
type PasskeyStore interface {
	ListAdminCredentials(ctx context.Context) ([]StoredAdminCredential, error)
	PutAdminCredential(ctx context.Context, credentialID, publicKey []byte, attestationType string, signCount uint32, transports []string) error
	UpdateAdminSignCount(ctx context.Context, dbID int64, count uint32) error
	DeleteAdminCredential(ctx context.Context, dbID int64) error
}

// StoredAdminCredential 存储行（含数据库 ID 与解码后的凭据）。
type StoredAdminCredential struct {
	DBID            int64
	CredentialID    []byte
	PublicKey       []byte
	AttestationType string
	SignCount       uint32
	Transports      []string
	CreatedAt       time.Time
}

// passkeyUser go-webauthn 的 User 适配（单管理员模型）。
type passkeyUser struct{ creds []webauthn.Credential }

func (u *passkeyUser) WebAuthnID() []byte { return []byte("githubhot-admin") }
func (u *passkeyUser) WebAuthnName() string { return "admin" }
func (u *passkeyUser) WebAuthnDisplayName() string { return "GithubHot 管理员" }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

type passkeyLoginSess struct {
	sd      *webauthn.SessionData
	creds   []webauthn.Credential
	dbIDs   map[string]int64 // credentialID(base64url) → 数据库行 ID
	expires time.Time
}

type passkeySessionStore struct {
	mu                sync.Mutex
	regSessions       map[string]*webauthn.SessionData // 管理会话 Cookie → 注册会话
	loginSessions     map[string]*passkeyLoginSess     // 一次性 token → 登录会话
}

func (s *Server) passkeyStoreRef() *passkeySessionStore {
	s.passkeyOnce.Do(func() { s.passkeyMaps = &passkeySessionStore{
		regSessions: map[string]*webauthn.SessionData{},
		loginSessions: map[string]*passkeyLoginSess{},
	} })
	return s.passkeyMaps
}

func (st *passkeySessionStore) sweepLoginLocked() {
	for k, v := range st.loginSessions {
		if time.Now().After(v.expires) {
			delete(st.loginSessions, k)
		}
	}
}

func webauthnEnabled() bool {
	return os.Getenv("WEBAUTHN_ENABLED") == "1" && strings.TrimSpace(os.Getenv("WEBAUTHN_RP_ID")) != "" &&
		strings.TrimSpace(os.Getenv("WEBAUTHN_ORIGIN")) != ""
}

func webauthnOnly() bool { return os.Getenv("WEBAUTHN_ONLY") == "1" }

// beginPasskeyRegister POST /api/v1/admin/passkey/begin-register（守卫组内）。
func (s *Server) beginPasskeyRegister(w http.ResponseWriter, r *http.Request) {
	if s.WebAuthn == nil || s.PasskeyStore == nil {
		writeErr(w, 503, errorString("WebAuthn 未启用（WEBAUTHN_ENABLED/RP_ID/ORIGIN）"))
		return
	}
	creds, _, err := s.listPasskeyCreds(r)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	user := &passkeyUser{creds: creds}
	options, sess, err := s.WebAuthn.BeginRegistration(user)
	if err != nil {
		writeErr(w, 500, errorString(err.Error()))
		return
	}
	ps := s.passkeyStoreRef()
	ps.mu.Lock()
	ps.regSessions[adminSessionFrom(r)] = sess
	ps.mu.Unlock()
	writeJSON(w, 200, options)
}

// finishPasskeyRegister POST /api/v1/admin/passkey/finish-register（守卫组内）。
func (s *Server) finishPasskeyRegister(w http.ResponseWriter, r *http.Request) {
	key := adminSessionFrom(r)
	ps := s.passkeyStoreRef()
	ps.mu.Lock()
	sess, ok := ps.regSessions[key]
	delete(ps.regSessions, key)
	ps.mu.Unlock()
	if !ok {
		writeErr(w, 400, errorString("未找到注册会话（请重新 begin-register）"))
		return
	}
	creds, _, err := s.listPasskeyCreds(r)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	cred, err := s.WebAuthn.FinishRegistration(&passkeyUser{creds: creds}, *sess, r)
	if err != nil {
		writeErr(w, 400, errorString("注册校验失败："+err.Error()))
		return
	}
	transports := make([]string, 0, len(cred.Transport))
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	if err := s.PasskeyStore.PutAdminCredential(r.Context(), cred.ID, cred.PublicKey,
		cred.AttestationType, cred.Authenticator.SignCount, transports); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok": true, "credential_id": base64.RawURLEncoding.EncodeToString(cred.ID),
		"transports": transports,
	})
}

// beginPasskeyLogin POST /api/v1/admin/passkey/begin-login（免密入口，守卫外）。
func (s *Server) beginPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	if s.WebAuthn == nil || s.PasskeyStore == nil {
		writeErr(w, 503, errorString("WebAuthn 未启用"))
		return
	}
	creds, dbIDs, err := s.listPasskeyCreds(r)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	if len(creds) == 0 {
		writeErr(w, 400, errorString("尚未注册任何通行密钥"))
		return
	}
	user := &passkeyUser{creds: creds}
	options, sess, err := s.WebAuthn.BeginLogin(user)
	if err != nil {
		writeErr(w, 500, errorString(err.Error()))
		return
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		writeErr(w, 500, errorString("生成登录令牌失败"))
		return
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	ps := s.passkeyStoreRef()
	ps.mu.Lock()
	ps.sweepLoginLocked()
	ps.loginSessions[token] = &passkeyLoginSess{sd: sess, creds: creds, dbIDs: dbIDs,
		expires: time.Now().Add(5 * time.Minute)}
	ps.mu.Unlock()
	writeJSON(w, 200, map[string]any{"token": token, "options": options})
}

// finishPasskeyLogin POST /api/v1/admin/passkey/finish-login（免密入口，守卫外）。
func (s *Server) finishPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	rawBody, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(rawBody)))
	var p struct {
		Token    string `json:"token"`
		Response struct {
			ID       string                  `json:"id"`
			RawID    protocol.URLEncodedBase64 `json:"rawId"`
			Type     string                  `json:"type"`
			Response json.RawMessage         `json:"response"`
		} `json:"response"`
	}
	if derr := json.NewDecoder(strings.NewReader(string(rawBody))).Decode(&p); derr != nil {
		writeErr(w, 400, errorString("请求体解析失败: "+derr.Error()))
		return
	}
	if p.Token == "" {
		writeErr(w, 400, errorString("请求体需要 {token, response}"))
		return
	}
	ps := s.passkeyStoreRef()
	ps.mu.Lock()
	sess, ok := ps.loginSessions[p.Token]
	if ok && time.Now().After(sess.expires) {
		delete(ps.loginSessions, p.Token)
		ok = false
	}
	if ok {
		delete(ps.loginSessions, p.Token) // 一次性消费
	}
	ps.mu.Unlock()
	if !ok {
		writeErr(w, 400, errorString("登录会话无效或已过期（重新 begin-login）"))
		return
	}
	ip := clientIPFromRequest(r)
	cred, err := s.WebAuthn.FinishLogin(&passkeyUser{creds: sess.creds}, *sess.sd, r)
	if err != nil {
		fmt.Printf("[passkey-dbg] finish err: %v | unwrapped: %v\n", err, errorsUnwrapAll(err))
		// 签名计数异常/断言不符 → 记录（错设备断言失败的验收点）
		if s.Guard != nil {
			s.Guard.Event(r.Context(), ip, "passkey-assert-fail", "通行密钥断言校验失败："+err.Error(), 10, false)
		}
		writeErr(w, 401, errorString("通行密钥校验失败："+err.Error()))
		return
	}
	if dbID, ok := sess.dbIDs[base64.RawURLEncoding.EncodeToString(cred.ID)]; ok {
		_ = s.PasskeyStore.UpdateAdminSignCount(r.Context(), dbID, cred.Authenticator.SignCount)
	}
	// 复用密码登录的会话通道（gh_admin_session，P0-2 IP 绑定同口径）
	sid := make([]byte, 32)
	if _, err := rand.Read(sid); err != nil {
		writeErr(w, 500, errorString("生成会话失败"))
		return
	}
	sessionID := base64.RawURLEncoding.EncodeToString(sid)
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	if err := s.Admin.CreateAdminSession(ctx, sessionID, ip, adminSessionTTL); err != nil {
		writeErr(w, 500, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: adminCookieName, Value: sessionID, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		Expires: time.Now().Add(adminSessionTTL),
	})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// listPasskeyCredentials GET /api/v1/admin/passkey/credentials（守卫组内）。
func (s *Server) listPasskeyCredentials(w http.ResponseWriter, r *http.Request) {
	creds, err := s.PasskeyStore.ListAdminCredentials(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	out := make([]map[string]any, 0, len(creds))
	for _, c := range creds {
		out = append(out, map[string]any{
			"id": c.DBID, "credential_id": base64.RawURLEncoding.EncodeToString(c.CredentialID),
			"attestation_type": c.AttestationType, "sign_count": c.SignCount,
			"transports": c.Transports, "created_at": c.CreatedAt,
		})
	}
	writeJSON(w, 200, map[string]any{"credentials": out})
}

// deletePasskeyCredential DELETE /api/v1/admin/passkey/credentials/{id}（守卫组内）。
func (s *Server) deletePasskeyCredential(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, errorString("非法 id"))
		return
	}
	if err := s.PasskeyStore.DeleteAdminCredential(r.Context(), id); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// listPasskeyCreds 存储行 → go-webauthn 凭据 + 行号映射（登录后回写签名计数用）。
func (s *Server) listPasskeyCreds(r *http.Request) ([]webauthn.Credential, map[string]int64, error) {
	rows, err := s.PasskeyStore.ListAdminCredentials(r.Context())
	if err != nil {
		return nil, nil, err
	}
	creds := make([]webauthn.Credential, 0, len(rows))
	dbIDs := map[string]int64{}
	for _, row := range rows {
		creds = append(creds, webauthn.Credential{
			ID:              row.CredentialID,
			PublicKey:       row.PublicKey,
			AttestationType: row.AttestationType,
			Authenticator: webauthn.Authenticator{SignCount: row.SignCount},
		})
		dbIDs[base64.RawURLEncoding.EncodeToString(row.CredentialID)] = row.DBID
	}
	return creds, dbIDs, nil
}

func errorsUnwrapAll(err error) string {
	parts := []string{}
	for e := err; e != nil; e = errors.Unwrap(e) {
		parts = append(parts, e.Error())
	}
	return strings.Join(parts, " <- ")
}

func adminSessionFrom(r *http.Request) string {
	if c, err := r.Cookie(adminCookieName); err == nil {
		return c.Value
	}
	return ""
}

// disablePasswordLoginIfOnly WEBAUTHN_ONLY=1 时 adminLogin 的拦截（在 adminLogin 开头调用）。
func passwordLoginDisabled() bool {
	return webauthnOnly() && webauthnEnabled()
}

var _ = protocol.ErrChallengeMismatch // 引用校验：库协议包可用性（编译期提示）
