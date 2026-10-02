package safehttp

import "testing"

func TestValidateURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"公网 https", "https://api.github.com/search", false},
		{"公网 http", "http://example.com/feed.xml", false},
		{"ftp 拒绝", "ftp://example.com/file", true},
		{"file 拒绝", "file:///etc/passwd", true},
		{"javascript 拒绝", "javascript:alert(1)", true},
		{"localhost 拒绝", "http://localhost:8080/api", true},
		{"*.localhost 拒绝", "http://api.localhost", true},
		{"*.local 拒绝", "http://printer.local/feed", true},
		{"*.internal 拒绝", "http://metadata.google.internal/computeMetadata/v1/", true},
		{"环回 IPv4 拒绝", "http://127.0.0.1:9090/", true},
		{"环回 IPv6 拒绝", "http://[::1]/", true},
		{"未指定地址拒绝", "http://0.0.0.0/", true},
		{"私网 10/8 拒绝", "http://10.1.2.3/", true},
		{"私网 172.16/12 拒绝", "http://172.16.0.9/", true},
		{"私网 192.168 拒绝", "http://192.168.1.1/", true},
		{"链路本地拒绝", "http://169.254.169.254/latest/meta-data/", true},
		{"CGNAT 拒绝", "http://100.64.0.1/", true},
		{"TEST-NET 拒绝", "http://192.0.2.1/", true},
		{"保留段 240/4 拒绝", "http://240.0.0.1/", true},
		{"IPv6 私有 fc00 拒绝", "http://[fc00::1]/", true},
		{"IPv6 链路本地拒绝", "http://[fe80::1]/", true},
		{"缺 host 拒绝", "http://", true},
		{"公网 IPv4 放行", "http://93.184.216.34/", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateURL(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("%s 应被拒绝", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("%s 应被放行: %v", tc.url, err)
			}
		})
	}
}

func TestValidateAndResolveLocalhostLiteral(t *testing.T) {
	// IP 字面量在 ValidateURL 就被拒，不会走到 DNS
	if _, err := ValidateAndResolve(testCtx(), "http://127.0.0.1/"); err == nil {
		t.Fatal("环回字面量应被拒绝")
	}
}
