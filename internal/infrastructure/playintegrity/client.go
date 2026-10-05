// Package playintegrity — P4-1 Google Play Integrity decodeIntegrityToken 客户端。
//
// 鉴权链：GCP service account JSON（env PLAY_INTEGRITY_SA_JSON 指向）→ 自签 RS256 JWT
// （scope = playintegrity）→ oauth2.googleapis.com/token 换 access_token →
// playintegrity.googleapis.com/v1/{package}:decodeIntegrityToken。
// 出站全部走 safehttp（SSRF 防护纪律）；JWT 用 stdlib crypto/rsa 自实现（无新依赖）。
package playintegrity

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

const (
	tokenURL   = "https://oauth2.googleapis.com/token"
	decodeBase = "https://playintegrity.googleapis.com/v1/%s:decodeIntegrityToken"
	// ScopeGoogleIntegrity OAuth scope（规格书 P4-1）。
	ScopeGoogleIntegrity = "https://www.googleapis.com/auth/playintegrity"
)

// SACredentials service account JSON 的最小字段集。
type SACredentials struct {
	ClientEmail  string `json:"client_email"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
}

// LoadSA 读取并解析 SA JSON 文件。
func LoadSA(path string) (*SACredentials, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sa SACredentials
	if err := json.Unmarshal(raw, &sa); err != nil {
		return nil, fmt.Errorf("SA JSON 解析失败: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, fmt.Errorf("SA JSON 缺少 client_email/private_key")
	}
	return &sa, nil
}

// Client Play Integrity 客户端。
type Client struct {
	sa        *SACredentials
	pkg       string
	accessToken string
	tokenExp  time.Time
}

func NewClient(sa *SACredentials, pkg string) *Client { return &Client{sa: sa, pkg: pkg} }

// rsaKey 惰性解析 PEM 私钥。
func (c *Client) rsaKey() (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(c.sa.PrivateKey))
	if block == nil {
		return nil, fmt.Errorf("SA 私钥 PEM 解析失败")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		if k, e2 := x509.ParsePKCS1PrivateKey(block.Bytes); e2 == nil {
			return k, nil
		}
		return nil, fmt.Errorf("SA 私钥解析失败: %w", err)
	}
	rk, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("SA 私钥不是 RSA")
	}
	return rk, nil
}

// mintJWT 自签 RS256 断言（iss=client_email, scope, aud=tokenURL）。
func (c *Client) mintJWT(now time.Time) (string, error) {
	key, err := c.rsaKey()
	if err != nil {
		return "", err
	}
	head := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := fmt.Sprintf(`{"iss":%q,"scope":%q,"aud":%q,"iat":%d,"exp":%d}`,
		c.sa.ClientEmail, ScopeGoogleIntegrity, tokenURL, now.Unix(), now.Add(time.Hour).Unix())
	body := head + "." + base64.RawURLEncoding.EncodeToString([]byte(claims))
	sum := sha256.Sum256([]byte(body))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return body + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// accessTokenCached 取 OAuth access token（带过期缓存）。
func (c *Client) accessTokenCached(ctx context.Context, now time.Time) (string, error) {
	if c.accessToken != "" && now.Before(c.tokenExp.Add(-time.Minute)) {
		return c.accessToken, nil
	}
	assertion, err := c.mintJWT(now)
	if err != nil {
		return "", err
	}
	form := "grant_type=" + enc("urn:ietf:params:oauth:grant-type:jwt-bearer") +
		"&assertion=" + enc(assertion)
	respBody, status, err := safehttp.Do(ctx, http.MethodPost, tokenURL,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, strings.NewReader(form))
	if err != nil {
		return "", fmt.Errorf("token 交换失败: %w", err)
	}
	if status != 200 {
		return "", fmt.Errorf("token 交换失败: HTTP %d", status)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &tok); err != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("token 响应解析失败")
	}
	c.accessToken = tok.AccessToken
	c.tokenExp = now.Add(time.Duration(tok.ExpiresIn) * time.Second)
	return c.accessToken, nil
}

// DecodeIntegrityToken 调 Google 解码 integrity token，返回 tokenPayloadExternal 原始 JSON。
func (c *Client) DecodeIntegrityToken(ctx context.Context, verdictToken string, now time.Time) ([]byte, error) {
	tok, err := c.accessTokenCached(ctx, now)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf(decodeBase, enc(c.pkg))
	payload := fmt.Sprintf(`{"integrity_token":%q}`, verdictToken)
	respBody, status, err := safehttp.Do(ctx, http.MethodPost, url, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + tok,
	}, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("decode 调用失败: %w", err)
	}
	if status != 200 {
		return nil, fmt.Errorf("decode 调用失败: HTTP %d", status)
	}
	return respBody, nil
}

func enc(s string) string {
	r := strings.NewReplacer(" ", "%20", ":", "%3A", "/", "%2F")
	return r.Replace(s)
}
