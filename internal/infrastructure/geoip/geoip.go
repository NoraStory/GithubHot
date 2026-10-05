// Package geoip — P2-5 GeoIP 交叉核验的数据读取层（规格书 §5 P2-5）。
//
// 数据源：sapics/ip-location-db 的 mmdb 文件（2026-06 起经 GitHub Releases 分发）：
//   - 国家库 dbip-country.mmdb（GEOIP_DB_PATH，默认 data/geo.mmdb）
//   - ASN 库 dbip-asn.mmdb（GEOIP_ASN_DB_PATH，默认 data/geo-asn.mmdb）
//
// 许可：DB-IP 数据 CC BY 4.0，Web 应用使用需保留 DB-IP.com 的署名链接（规格书 §12
// 致谢要求）。规格书原定的 geo-whois-asn-country 数据集已被上游下线（WHOIS 合规），
// 换用同许可的 DBIP 系（实测调整，见交付说明）。
//
// 实测调整：DBIP release 格式的 databaseType 是 "country ipvAll"/"asn ipvAll"，
// geoip2-golang 的类型化读取器会拒绝（"reader does not support ..."）——因此改用其
// 底层 maxminddb-golang 做宽容字段解码（同族纯 Go、CGO=0），DBIP 与 GeoLite2 两种
// schema 通用。
//
// 纪律：文件缺失 → 对应能力静默禁用（不报错、不阻塞启动），全部地理核验降级跳过；
// 读取器进程内复用（mmdb 打开后常驻内存映射）。
package geoip

import (
	"net"
	"os"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

// DownloadURLs geo download 子命令拉取的官方直链（免账号，Releases/latest 常青链接）。
const (
	CountryURL = "https://github.com/sapics/ip-location-db/releases/download/latest/dbip-country.mmdb"
	ASNURL     = "https://github.com/sapics/ip-location-db/releases/download/latest/dbip-asn.mmdb"
)

// DefaultPaths 从环境读库路径：GEOIP_DB_PATH / GEOIP_ASN_DB_PATH（缺省 data/ 下）。
func DefaultPaths() (country, asn string) {
	country = getenv("GEOIP_DB_PATH", "data/geo.mmdb")
	asn = getenv("GEOIP_ASN_DB_PATH", "data/geo-asn.mmdb")
	return country, asn
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

// countryRecord / asnRecord 宽容解码结构：同时兼容两种 schema——
//   - DBIP release 格式：顶层 country_code（实测 "country ipvAll" 库）
//   - GeoLite2/GeoIP2 格式：country.iso_code
//
// maxminddb 标签按 mmdb 数据键匹配，缺失键为零值，不报错。
type countryRecord struct {
	CountryCode string `maxminddb:"country_code"`
	Country     struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

type asnRecord struct {
	AutonomousSystemNumber       uint32 `maxminddb:"autonomous_system_number"`
	AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
}

// Service 国家/ASN 读取器。任一库缺失 → 对应方法返回零值，调用方按"无数据"跳过核验。
type Service struct {
	country *maxminddb.Reader
	asn     *maxminddb.Reader
}

var (
	openMu sync.Mutex
	// opened 按路径缓存已打开的读取器：引擎与 CLI 共用一套进程内实例。
	opened = map[string]*maxminddb.Reader{}
)

func openReader(path string) *maxminddb.Reader {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil // 文件缺失：静默禁用（离线部署纪律，见规格书 P2-5）
	}
	openMu.Lock()
	defer openMu.Unlock()
	if r, ok := opened[path]; ok {
		return r
	}
	r, err := maxminddb.Open(path)
	if err != nil {
		return nil
	}
	opened[path] = r
	return r
}

// Open 打开两个库；路径为空或文件不存在 → 对应能力禁用。
func Open(countryPath, asnPath string) *Service {
	return &Service{country: openReader(countryPath), asn: openReader(asnPath)}
}

// Enabled 是否至少有一个库可用。
func (s *Service) Enabled() bool { return s != nil && (s.country != nil || s.asn != nil) }

// Country 返回 IP 的 ISO 国家码（大写两位）；无库/未知 → ""。
func (s *Service) Country(ip net.IP) string {
	if s == nil || s.country == nil || ip == nil {
		return ""
	}
	var rec countryRecord
	if err := s.country.Lookup(ip, &rec); err != nil {
		return ""
	}
	if rec.CountryCode != "" {
		return strings.ToUpper(rec.CountryCode)
	}
	return strings.ToUpper(rec.Country.IsoCode)
}

// ASN 返回 IP 的自治系统号与组织名；无库/未知 → (0, "")。
func (s *Service) ASN(ip net.IP) (uint, string) {
	if s == nil || s.asn == nil || ip == nil {
		return 0, ""
	}
	var rec asnRecord
	if err := s.asn.Lookup(ip, &rec); err != nil {
		return 0, ""
	}
	return uint(rec.AutonomousSystemNumber), rec.AutonomousSystemOrganization
}

// hostingKeywords 数据中心/云厂商组织名关键词（小写匹配）。宁可漏判不可误判：
// 只收"几乎不可能是家宽/手机出口"的托管商；Google/微软等消费级 ISP 同名组织
// 用更精确的字样（google cloud / microsoft corporation）区分。
var hostingKeywords = []string{
	"amazon", "aws", "google cloud", "microsoft corporation", "azure", "oracle cloud",
	"digitalocean", "ovh", "hetzner", "linode", "vultr", "choopa", "constant company",
	"leaseweb", "contabo", "scaleway", "datacamp", "m247", "gcore", "alibaba", "tencent cloud",
	"huawei cloud", "upcloud", "packethub", "packethost",
}

// HostingOrg 组织名是否命中数据中心/云厂商（ASN 类型判定的启发式，规格书 P2-5-③）。
func HostingOrg(org string) bool {
	o := strings.ToLower(org)
	if o == "" {
		return false
	}
	for _, k := range hostingKeywords {
		if strings.Contains(o, k) {
			return true
		}
	}
	return false
}

// countryContinents 国家 → 大洲集合。跨洲国家列出全部可能大洲（宽松判定，
// 只有"时区大洲完全不在集合内"才算跨洲级不符——规格书：邻区忽略）。
var countryContinents = map[string][]string{
	"CN": {"AS"}, "JP": {"AS"}, "KR": {"AS"}, "KP": {"AS"}, "IN": {"AS"}, "ID": {"AS"},
	"VN": {"AS"}, "TH": {"AS"}, "MY": {"AS"}, "SG": {"AS"}, "PH": {"AS"}, "HK": {"AS"},
	"MO": {"AS"}, "TW": {"AS"}, "PK": {"AS"}, "BD": {"AS"}, "LK": {"AS"}, "NP": {"AS"},
	"KZ": {"AS", "EU"}, "UZ": {"AS"}, "SA": {"AS"}, "AE": {"AS"}, "QA": {"AS"}, "IL": {"AS"},
	"IR": {"AS"}, "IQ": {"AS"}, "TR": {"AS", "EU"},
	"RU": {"EU", "AS"},
	"DE": {"EU"}, "FR": {"EU"}, "GB": {"EU"}, "IT": {"EU"}, "ES": {"EU"}, "PT": {"EU"},
	"NL": {"EU"}, "BE": {"EU"}, "PL": {"EU"}, "CZ": {"EU"}, "AT": {"EU"}, "CH": {"EU"},
	"SE": {"EU"}, "NO": {"EU"}, "DK": {"EU"}, "FI": {"EU"}, "IE": {"EU"}, "GR": {"EU"},
	"RO": {"EU"}, "HU": {"EU"}, "UA": {"EU"}, "BY": {"EU"},
	"US": {"NA"}, "CA": {"NA"}, "MX": {"NA"}, "GT": {"NA"}, "CR": {"NA"}, "PA": {"NA"},
	"CU": {"NA"}, "DO": {"NA"}, "HT": {"NA"}, "HN": {"NA"}, "NI": {"NA"}, "SV": {"NA"},
	"BR": {"SA"}, "AR": {"SA"}, "CL": {"SA"}, "CO": {"SA"}, "PE": {"SA"}, "UY": {"SA"},
	"VE": {"SA"}, "EC": {"SA"}, "BO": {"SA"}, "PY": {"SA"},
	"ZA": {"AF"}, "NG": {"AF"}, "EG": {"AF"}, "KE": {"AF"}, "MA": {"AF"}, "DZ": {"AF"},
	"GH": {"AF"}, "TZ": {"AF"}, "ET": {"AF"}, "UG": {"AF"},
	"AU": {"OC"}, "NZ": {"OC"}, "FJ": {"OC"}, "PG": {"OC"},
}

// tzContinentOverrides IANA 时区首段 ≠ 大洲名的特例（无大洲信息 → 跳过核验）。
var tzContinentOverrides = map[string]string{
	"etc": "",
	"utc": "",
	"gmt": "",
}

// TZContinent 从 IANA 时区名提取大洲（首段），如 Asia/Shanghai → "AS"。
// 无大洲段（UTC/GMT/Etc/*）→ ""，调用方跳过核验。
func TZContinent(tz string) string {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return ""
	}
	first := strings.ToLower(strings.SplitN(tz, "/", 2)[0])
	if ov, ok := tzContinentOverrides[first]; ok {
		return ov
	}
	switch first {
	case "asia":
		return "AS"
	case "europe":
		return "EU"
	case "africa":
		return "AF"
	case "america", "us", "canada", "mexico", "brazil":
		return "NA"
	case "australia", "pacific", "nz":
		return "OC"
	case "atlantic":
		return "EU" // 亚速尔/加那利等欧洲属地
	case "indian":
		return "AF"
	case "antarctica":
		return "" // 科考站时区不可判定，跳过
	default:
		return ""
	}
}

// ContinentMismatch 客户端时区大洲是否与 IP 归属国跨洲不符（规格书：跨洲才计分）。
// 任一侧信息缺失 → false（不判定）。
func ContinentMismatch(countryISO, tz string) bool {
	continents, ok := countryContinents[strings.ToUpper(countryISO)]
	if !ok || len(continents) == 0 {
		return false // 未知国家：不判定
	}
	tzc := TZContinent(tz)
	if tzc == "" {
		return false
	}
	for _, c := range continents {
		if c == tzc {
			return false
		}
	}
	return true
}
