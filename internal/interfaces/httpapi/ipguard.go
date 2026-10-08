package httpapi

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"log"
	"math"
	mathrand "math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/behavior"
	"github.com/NoraStory/GithubHot/internal/domain/fpcluster"
	"github.com/NoraStory/GithubHot/internal/domain/fpmath"
	"github.com/NoraStory/GithubHot/internal/domain/iprisk"
	"github.com/NoraStory/GithubHot/internal/infrastructure/geoip"
	"github.com/NoraStory/GithubHot/internal/infrastructure/ja4db"
)

// ---------- 三层 IP 身份识别体系 ----------
//
// 第一层（IP 记忆）：全站中间件按 IP 建滑动窗口档案（速率/404 率/UA 集合），
//   异常记入违规事件并轻量落库 ip_profiles。
// 第二层（设备指纹）：浏览器采集 Canvas/WebGL/音频/字体/WebRTC/屏幕/环境特征上报，
//   服务端维护 ip_fingerprints 的 fp↔IP↔UA↔分量指纹多对多关系。
// 第三层（特殊标识 + 一致性核验）：对通过初检的访客签发 HMAC 身份令牌
//   （gh_id Cookie，绑定 IP+指纹+有效期）；此后每次 API 请求带 X-Device-Fp，
//   服务端核验签名完整性、IP↔设备绑定、环境自洽；10 分钟违规积分 ≥100 触发
//   升级封禁（30m→24h→7d→30d，30 天无违规衰减回初犯档），被封 IP 全站 404。
//
// 防误封算法（出差/换网络场景，双因子原则）：
//   1. 换 IP/换网络本身永不直接封禁——身份类信号（drift/churn/连坐）必须与
//      "该设备自身有劣迹"叠加才升级；干净设备连到被封的共享出口（酒店/机场/
//      运营商 NAT）只记低分观察。
//   2. 速率类信号按 UA 分档：真人浏览器共享出口（公司 NAT）30 分，脚本 UA 60 分，
//      超 3 倍阈值不分档。
//   3. 同一设备换网络（指纹与令牌绑定一致）的 IP 漂移只记录不计分；
//      无指纹/指纹不符（Cookie 被搬）才按高危 60 分。
//   4. 首犯一律 30 分钟短封自动解封（误封成本有上限），管理端会话 IP 临时白名单。
//
// 回环/内网默认白名单（IP_GUARD_LOCAL=1），公网部署设 IP_GUARD_LOCAL=0。

const (
	idCookieName     = "gh_id"
	idCookieLifetime = 7 * 24 * time.Hour
	fpHeaderName     = "X-Device-Fp"

	windowSize      = 5 * time.Minute
	profileFlushTTL = 60 * time.Second
	eventDedupeTTL  = 5 * time.Minute
	fpReportMaxRPM  = 12

	scoreBanThreshold = 100
	banWindowSeconds  = 600

	// 严重违规：直接按 7 天档
	severeRatePerMin = 600
	// 普通阈值
	warnRatePerMin  = 150
	adminRatePerMin = 30
	scannerMinReqs  = 50
	scanner404Ratio = 0.4
	// 指纹全生命周期关联 IP 数超此值记漂移观察（出差多年累积也难触及）
	fpChurnMaxIPs = 12
	// 连坐/漂移判定"设备劣迹"的时间窗与抽查 IP 数
	fpViolationLookback = 7 * 24 * 3600
	fpViolationProbe    = 5
	// 封禁累犯衰减期：超过该时长无违规，strike 回初犯档
	banStrikeDecay = 30 * 24 * time.Hour

	// P2-1 感知哈希关联扫描：30 天窗口、单次最多取 2000 条候选
	phashLinkWindow     = 30 * 24 * time.Hour
	phashCandidateLimit = 2000

	// P2-2 MinHash+LSH：签名 128 位 = 16 带 × 8 行；召回上限与精确 Jaccard 关联阈值
	minhashCandidateLimit = 200
	minhashJaccardMin     = 0.8
	// P2-3 熵值加权：封禁触发系数 = min(1, entropy_bits/40)；每日刷新窗口与行数上限
	entropyFactorBits = 40.0
	entropyWindow     = 30 * 24 * time.Hour
	entropyMaxRows    = 20000
	// P2-4 稳定性 EWMA（α=0.3）与"历史稳定"判定线
	stabilityAlpha     = 0.3
	stabilityStableMin = 0.7
	rotationOldMinAge  = time.Hour
	// P2-5 GeoIP 计分项（灰度期 FP_SCORE_SHADOW 下全部 0 分只记录）
	geoTZMismatchScore      = 10
	geoHostingMobileUAScore = 15
	// P2-4 轮换检测计分（换脸实锤：稳定分量突变 + 数学指纹关联旧指纹）
	rotationDetectedScore = 25
	// P3-3 UA↔TLS 交叉核验：浏览器 UA + 已知非浏览器 TLS 栈（高置信）
	uaTLSMismatchScore = 25
	// P4-5 行为机器特征计分（完全匀速/纯直线/机械击键，灰度 0 分）
	behaviorMachineScore = 15

	// 握手通道专用限流：/api/v1/site/config 免签（APP 冷启动要从这里拿远程封禁 /
	// 强制更新策略）且会触发指纹归档写库，必须自己限流，否则可被无限重放。
	handshakePath        = "/api/v1/site/config"
	handshakeRateDefault = 30 // req/min/ip
)

// rotationWatchKeys P2-4 轮换检测的设备级稳定分量（字体清单/显卡标识——
// 正常驱动漂移不会动它们；切片不能进 const 块）。
var rotationWatchKeys = []string{"fonts", "webgl", "renderer"}

// sanitizeForLog 净化字符串以防止日志注入攻击
func sanitizeForLog(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	// 截断过长的字符串
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// init 验证常量边界
func init() {
	if stabilityAlpha <= 0 || stabilityAlpha >= 1 {
		panic("stabilityAlpha must be in (0, 1)")
	}
	if fpReportMaxRPM <= 0 || fpReportMaxRPM > 100 {
		panic("fpReportMaxRPM must be in (0, 100]")
	}
	if handshakeRateDefault <= 0 || handshakeRateDefault > 1000 {
		panic("handshakeRateDefault must be in (0, 1000]")
	}
}

// GuardStore 防护存储端口（sqlite.DB 实现，cli 层适配）。
type GuardStore interface {
	AddIPEvent(ctx context.Context, ip, kind, detail string, score int) error
	RecentIPEventsScore(ctx context.Context, ip string, seconds int) (int, error)
	ListIPEvents(ctx context.Context, limit int) ([]IPEventDTO, error)
	ListIPEventsSince(ctx context.Context, limit int, since time.Time) ([]IPEventDTO, error)
	UpsertFingerprint(ctx context.Context, fp, ip, ua string, meta FingerprintMeta) ([]string, error)
	ListFingerprints(ctx context.Context, limit int) ([]FingerprintDTO, error)
	ListFingerprintsSince(ctx context.Context, since time.Time, limit int) ([]FingerprintDTO, error)
	// P2-1/P2-2 关联：感知哈希候选扫描与关联边读写（P4-4 聚类、P6 图快照共用）
	ListPHashCandidates(ctx context.Context, since time.Time, limit int) ([]PHashRowDTO, error)
	UpsertFPLink(ctx context.Context, src, dst, kind string, weight float64) error
	ListFPLinks(ctx context.Context, fp string, limit int) ([]FPLinkDTO, error)
	// P2-2 LSH 桶读写与候选召回；FindFingerprint 供稳定性/轮换/熵权读取单条档案
	UpsertLSHBands(ctx context.Context, fp string, bands []LSHBand) error
	ListLSHCandidates(ctx context.Context, fp string, bands []LSHBand, limit int) ([]string, error)
	ListMinHashSigs(ctx context.Context, fps []string) ([]MinHashSigRow, error)
	FindFingerprint(ctx context.Context, fp string) (*FingerprintDTO, error)
	// P2-3 熵权每日刷新；P2-5 IP 档案地理信息补全；P4-1 平台证明结果写回
	UpdateEntropyBits(ctx context.Context, fp string, bits float64) error
	UpdateIPGeo(ctx context.Context, ip string, asn uint, asnType, country, tz string) error
	UpdateAttestation(ctx context.Context, fp string, attestationJSON string) error
	UpdateAnomalyScore(ctx context.Context, fp string, score float64) error
	UpdateGNN(ctx context.Context, fp string, score float64, embeddingJSON string) error
	AddReviewItem(ctx context.Context, fp string, reasons []string, scores map[string]float64) error
	ListReviewItems(ctx context.Context, status string, limit int) ([]ReviewItemRow, error)
	ResolveReviewItems(ctx context.Context, ids []int64, action string, resolvedBy string) error
	CountReviewItems(ctx context.Context, status string) (int, error)
	// P4-4 图聚类：每日 cron 全量重算簇并写回 cluster_id / fp_clusters
	ReplaceClusters(ctx context.Context, clusters []ClusterDTO) error
	ListAllFPLinks(ctx context.Context, since time.Time, limit int) ([]FPLinkDTO, error)
	ListClusters(ctx context.Context, limit int) ([]ClusterDTO, error)
	// P5-1 标注体系：金标签写回 + 弱标签每日重建 + 训练导出
	UpsertFpLabel(ctx context.Context, fp, label, source string, confidence float64, notes string) error
	ListFpLabels(ctx context.Context) ([]FpLabelRow, error)
	BestFpLabels(ctx context.Context) (map[string]struct {
		Label      string
		Confidence float64
	}, error)
	DeleteFpRuleLabels(ctx context.Context) error
	FindBan(ctx context.Context, ip string) (*BanDTO, error)
	BannedAmong(ctx context.Context, ips []string) ([]string, error)
	UpsertBan(ctx context.Context, ip string, strikes, level int, reason string, duration time.Duration) error
	ListBans(ctx context.Context) ([]BanDTO, error)
	DeleteBan(ctx context.Context, ip string) error
	TouchIPProfile(ctx context.Context, ip, ua string, reqs int) error
	FindIPProfile(ctx context.Context, ip string) (*IPProfileDTO, error)
	ListFingerprintsByIP(ctx context.Context, ip string, limit int) ([]FingerprintDTO, error)
	ListIPEventsByIP(ctx context.Context, ip string, limit int) ([]IPEventDTO, error)
}

// DTO（存储层与接口层解耦）。
type IPEventDTO struct {
	ID     int64
	IP     string
	Kind   string
	Detail string
	Score  int
	At     time.Time
}
type FingerprintDTO struct {
	Fingerprint   string
	IPs           []string
	Webrtc        []string
	Components    map[string]string
	Flags         []string
	UA            string
	FirstSeen     time.Time
	LastSeen      time.Time
	Hits          int
	CanvasPHash   string             // P2-1 感知哈希（hex16）
	MinHashSig    string             // P2-2 组件集合 MinHash 签名（hex）
	EntropyBits   float64            // P2-3 分量熵权（信息量 bit，每日 cron 刷新；0=未计算）
	Stability     float64            // P2-4 整体时间稳定度（0-1）
	CompStability map[string]float64 // P2-4 各分量稳定度（键 → EWMA，缺失键 = 无历史）
	JA4           string             // P3-2 TLS 客户端指纹（TLS 模式下捕获；纯 HTTP 为空）
	Attestation   string             // P4-1 平台证明结果（原始 JSON，'{}'=未验证）
	BehaviorJSON  string             // P4-5 行为生物特征（滑窗统计量 JSON，'{}'=未采集）
	ClockSkewPPM  *float64           // P4-6 时钟偏移（ppm；NULL=未采集）
	ClusterID     *int64             // P4-4 图聚类簇归属（NULL=未聚类）
	GNNScore      *float64           // P6-3 GNN 推理 bot 概率（NULL=未打分）
	AnomalyScore  *float64           // P5-2 iForest 异常分（NULL=未计算）
} // PHashRowDTO 感知哈希候选行（同源关联扫描）。
type PHashRowDTO struct {
	Fingerprint string
	PHash       string
	LastSeen    time.Time
}

// MinHashSigRow 候选指纹的 MinHash 签名（LSH 召回后精确 Jaccard 用）。
type MinHashSigRow struct {
	FP  string
	Sig string
}

// LSHBand 一条 LSH 桶定位（band 号 + 桶键，见 fpmath.Bands）。
type LSHBand struct {
	Band int
	Hash string
}

// ReviewItemRow review_items 行（§10.2）。
type ReviewItemRow struct {
	ID         int64
	FP         string
	Reasons    []string
	Scores     map[string]float64
	Status     string
	CreatedAt  time.Time
	ResolvedAt *time.Time
	ResolvedBy string
}

// FpLabelRow 标注行（P5-1）。
type FpLabelRow struct {
	FP         string
	Label      string // human | bot | uncertain
	Source     string // admin | rule | model
	Confidence float64
	LabeledAt  time.Time
	Notes      string
}

// FPLinkDTO 指纹关联边。
type FPLinkDTO struct {
	Src, Dst, Kind      string
	Weight              float64
	FirstSeen, LastSeen time.Time
}

// ClusterDTO 簇档案（P4-4 聚类写回与集群视图）。
type ClusterDTO struct {
	ID        int64     `json:"id"`
	Members   []string  `json:"members"`
	Size      int       `json:"size"`
	FirstSeen time.Time `json:"first_seen"`
	Reason    string    `json:"reason"`
}

// FingerprintMeta 指纹上报的附带信息（WebRTC IP、分量明细、环境核验命中、P2 数学指纹）。
type FingerprintMeta struct {
	Webrtc        []string
	Components    map[string]string
	Flags         []string
	CanvasPHash   string              // P2-1 64bit 感知哈希（hex16），可选
	MinHashSig    string              // P2-2 组件集合 MinHash 签名（hex），可选（服务端计算）
	Sets          map[string][]string // P2-2 原始清单（fonts/webgl_exts/plugins，仅用于算签名，不入库）
	TZ            string              // P2-5 客户端 IANA 时区（Asia/Shanghai），可选
	TZOffsetMin   int                 // P2-5 客户端时区偏移（分钟，东八区=480），可选
	Stability     float64             // P2-4 整体稳定度（引擎计算后随 upsert 落库）
	CompStability map[string]float64  // P2-4 各分量稳定度（引擎计算后随 upsert 落库）
	JA4           string              // P3-2 TLS 客户端指纹（TLS 模式下由连接上下文注入）
	Behavior      string              // P4-5 行为生物特征 JSON（客户端滑窗统计量，已清洗）
	ClockSkewPPM  *float64            // P4-6 时钟偏移（ppm；nil=未采集）
	Trusted       bool                // 渗透修复：来源可信（有效 gh_id 会话或已验签 APP）——不可信上报不得覆写已有指纹的 components 基线
}

// ja4CtxKey TLS 指纹的 context 键（连接级注入，请求级读取）。
type ja4CtxKey struct{}

// JA4Resolver 延迟解析连接的 JA4：Go 的 TLS 握手是惰性的（首个请求读取时才发生），
// ConnContext 执行时握手尚未开始、指纹还没算出来——所以上下文里放"解析器引用"，
// 请求时刻（握手已完成）再取。
type JA4Resolver interface {
	ResolveJA4() string
}

// WithJA4 注入 JA4：v 为字符串（已知值，测试/直连场景）或 JA4Resolver（延迟解析）。
func WithJA4(ctx context.Context, v any) context.Context {
	return context.WithValue(ctx, ja4CtxKey{}, v)
}

// JA4FromContext 读取当前连接的 JA4 指纹（非 TLS 模式返回空串）。
func JA4FromContext(ctx context.Context) string {
	switch v := ctx.Value(ja4CtxKey{}).(type) {
	case string:
		return v
	case JA4Resolver:
		return v.ResolveJA4()
	}
	return ""
}

type BanDTO struct {
	IP        string
	Strikes   int
	Level     int
	Reason    string
	BannedAt  time.Time
	ExpiresAt time.Time
}
type IPProfileDTO struct {
	IP        string
	FirstSeen time.Time
	LastSeen  time.Time
	Reqs      int
	UASet     []string
	UALast    string
}

// banDuration 按违规次数定封禁时长（升级制）。
// 优化版：防止负数溢出导致封禁时长错误。
func banDuration(strikes int) time.Duration {
	// 防御：负数或零按初犯处理
	if strikes < 1 {
		strikes = 1
	}
	switch {
	case strikes <= 1:
		return 30 * time.Minute
	case strikes == 2:
		return 24 * time.Hour
	case strikes == 3:
		return 7 * 24 * time.Hour
	default:
		return 30 * 24 * time.Hour
	}
}

// ipWindow 单 IP 的内存滑动档案（第一层）。
type ipWindow struct {
	times     []time.Time // 最近 5 分钟请求时间
	notFound  int         // 窗口内 404 数
	reqs      int         // 本小时累计
	uaSet     map[string]bool
	lastFlush time.Time
	dirty     bool
	hsTimes   []time.Time // 握手通道请求时间（独立 60s 窗口，见 handshakeAllow）
}

// GeoProvider P2-5 地理数据源（geoip.Service 实现；测试可注入 stub）。
type GeoProvider interface {
	Enabled() bool
	Country(ip net.IP) string
	ASN(ip net.IP) (uint, string)
}

// JA4Mapper P3-3 JA4 → 应用名 映射源（ja4db.DB 实现；测试可注入 stub）。
type JA4Mapper interface {
	Loaded() bool
	Lookup(ja4 string) (app string, ok bool)
}

// IPGuard 防护引擎。
type IPGuard struct {
	store    GuardStore
	key      []byte // HMAC 密钥
	geo      GeoProvider
	ja4      JA4Mapper
	mlEngine *MLEngine // 统一ML调度器（P5-2 iForest + P5-3 LR + P6-3 GNN）

	enabled bool
	localOK bool // 回环/内网放行

	mu           sync.Mutex
	windows      map[string]*ipWindow
	talkers      map[string]int // 本小时请求计数（整点重置）
	talkersReset time.Time
	dedupe       map[string]time.Time // ip+kind → 上次事件时间
	banCache     map[string]banCacheEntry
	fpReport     map[string][]time.Time // 指纹上报限频
	// whitelist 管理端会话 IP 临时免封禁（登录/会话校验时刷新，TTL 与会话一致）
	whitelist map[string]time.Time
	// fpColl 指纹碰撞检测（同型号设备指纹重合 → 豁免连坐）
	fpColl *fpCollision

	// 清理时间戳（防止内存泄漏）
	lastDedupeClean    time.Time
	lastFpReportClean  time.Time
	lastWhitelistClean time.Time
}

type banCacheEntry struct {
	banned  bool
	expires time.Time
	at      time.Time
}

// NewIPGuard 构建引擎；secret 为空时进程内随机生成（重启后旧令牌失效）。
// GeoIP 数据库文件缺失时地理核验整体降级（规格书 P2-5：不报错不阻塞）。
func NewIPGuard(store GuardStore) *IPGuard {
	secret := os.Getenv("IP_GUARD_SECRET")
	key := []byte(secret)
	if len(key) < 32 {
		key = make([]byte, 32)
		n, err := rand.Read(key)
		if err != nil {
			log.Fatalf("[ipguard] CRITICAL: 随机密钥生成失败: %v", err)
		}
		if n != 32 {
			log.Fatalf("[ipguard] CRITICAL: 随机密钥长度不足: %d/32", n)
		}
		log.Printf("[ipguard] 警告: IP_GUARD_SECRET 未设置或过短，已生成随机密钥（重启后令牌失效）")
	}
	countryPath, asnPath := geoip.DefaultPaths()
	now := time.Now()

	// 创建ML引擎
	mlEngine := NewMLEngine(MLConfig{
		ModelDir:          os.Getenv("ML_MODEL_DIR"),
		EnableIForest:     os.Getenv("ML_ENABLE_IFOREST") != "0",
		EnableLR:          os.Getenv("ML_ENABLE_LR") != "0",
		IForestInterval:   24 * time.Hour,
		IForestMinSamples: 10,
		IForestMaxSamples: 10000,
		LRCheckInterval:   1 * time.Minute,
	})

	g := &IPGuard{
		store:              store,
		key:                key,
		geo:                geoip.Open(countryPath, asnPath),
		enabled:            os.Getenv("IP_GUARD_ENABLED") != "0",
		localOK:            os.Getenv("IP_GUARD_LOCAL") != "0",
		windows:            map[string]*ipWindow{},
		talkers:            map[string]int{},
		talkersReset:       now.Truncate(time.Hour).Add(time.Hour),
		dedupe:             map[string]time.Time{},
		banCache:           map[string]banCacheEntry{},
		fpReport:           map[string][]time.Time{},
		whitelist:          map[string]time.Time{},
		fpColl:             newFPCollision(),
		mlEngine:           mlEngine,
		lastDedupeClean:    now,
		lastFpReportClean:  now,
		lastWhitelistClean: now,
	}

	// 启动后台任务
	go g.periodicCleanup()
	mlEngine.Start(g) // 启动ML调度器

	if g.geo.Enabled() {
		log.Printf("[ipguard] GeoIP 已启用（%s）", countryPath)
	}
	log.Printf("[ipguard] ML引擎已启动（iForest=%v, LR=%v）", mlEngine.cfg.EnableIForest, mlEngine.cfg.EnableLR)
	return g
}

// isLocalIP 回环/内网判定。
func isLocalIP(ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return false
	}
	return p.IsLoopback() || p.IsPrivate() || p.IsLinkLocalUnicast()
}

// periodicCleanup 定期清理内存map（防止内存泄漏，Critical修复）
func (g *IPGuard) periodicCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		g.mu.Lock()

		// 1. 清理过期 dedupe（5分钟TTL，保留2倍窗口）
		if len(g.dedupe) > 10000 || now.Sub(g.lastDedupeClean) > 10*time.Minute {
			cutoff := now.Add(-2 * eventDedupeTTL)
			cleaned := 0
			for k, t := range g.dedupe {
				if t.Before(cutoff) {
					delete(g.dedupe, k)
					cleaned++
				}
			}
			g.lastDedupeClean = now
			if cleaned > 0 {
				log.Printf("[ipguard] 清理过期去重项: %d 条", cleaned)
			}
		}

		// 2. 清理闲置 fpReport（10分钟无活动）
		if len(g.fpReport) > 50000 || now.Sub(g.lastFpReportClean) > 10*time.Minute {
			cutoff := now.Add(-10 * time.Minute)
			cleaned := 0
			for ip, times := range g.fpReport {
				if len(times) == 0 || times[len(times)-1].Before(cutoff) {
					delete(g.fpReport, ip)
					cleaned++
				}
			}
			g.lastFpReportClean = now
			if cleaned > 0 {
				log.Printf("[ipguard] 清理闲置指纹上报记录: %d 条", cleaned)
			}
		}

		// 3. 清理过期 whitelist
		if len(g.whitelist) > 1000 || now.Sub(g.lastWhitelistClean) > time.Hour {
			cleaned := 0
			for ip, exp := range g.whitelist {
				if now.After(exp) {
					delete(g.whitelist, ip)
					cleaned++
				}
			}
			g.lastWhitelistClean = now
			if cleaned > 0 {
				log.Printf("[ipguard] 清理过期白名单: %d 条", cleaned)
			}
		}

		// 4. 清理闲置 windows（超过30分钟无活动且无请求记录）
		if len(g.windows) > 50000 {
			cutoff := now.Add(-30 * time.Minute)
			cleaned := 0
			for ip, w := range g.windows {
				if len(w.times) == 0 && w.lastFlush.Before(cutoff) {
					delete(g.windows, ip)
					cleaned++
					if cleaned >= 5000 {
						break // 每轮最多清理5000条
					}
				}
			}
			if cleaned > 0 {
				log.Printf("[ipguard] 清理闲置IP窗口: %d 条", cleaned)
			}
		}

		// 5. 清理老化 banCache（超过10分钟的条目）
		if len(g.banCache) > 10000 {
			cutoff := now.Add(-10 * time.Minute)
			cleaned := 0
			for ip, entry := range g.banCache {
				if entry.at.Before(cutoff) {
					delete(g.banCache, ip)
					cleaned++
				}
			}
			if cleaned > 0 {
				log.Printf("[ipguard] 清理老化封禁缓存: %d 条", cleaned)
			}
		}

		g.mu.Unlock()

		// 记录内存统计（每10分钟）
		if now.Minute()%10 == 0 {
			g.mu.Lock()
			log.Printf("[ipguard] 内存统计: windows=%d talkers=%d dedupe=%d banCache=%d fpReport=%d whitelist=%d",
				len(g.windows), len(g.talkers), len(g.dedupe), len(g.banCache), len(g.fpReport), len(g.whitelist))
			g.mu.Unlock()
		}
	}
}

// clientIPFromRequest 见 clientip.go（受信代理解析：默认不信任 X-Forwarded-For）。

// ---------- 身份令牌（第三层·特殊标识） ----------
//
// 令牌格式（渗透测试 M-4 修复）：
//   v3.<b64url(nonce ‖ AES-256-GCM 密文)>   —— 现行。载荷（ip|fp|issued|exp）AES-256-GCM
//      加密：Cookie 被窃取者无法解码 fp，跨 IP 零分接管路径（换网络宽限需 fp 与令牌
//      绑定一致）失去 fp 来源。密钥 = SHA-256(key ‖ "gh-id-aead-v3")，与验签密钥隔离。
//   v2.<b64url(载荷)>.<b64url(HMAC)>          —— 旧版（明文+HMAC），签发已停用，
//      核验保留兼容（存量会话平滑过渡，不把老访客打成封禁）。
//
// fp 为签发时绑定的设备指纹，空串表示页面导航类请求（尚未采集指纹）。

// idAEADKey AEAD 子密钥：与 HMAC 验签密钥域隔离（域分隔符防跨用途重用）。
func idAEADKey(key []byte) []byte {
	sum := sha256.Sum256(append(append([]byte{}, key...), []byte("gh-id-aead-v3")...))
	return sum[:]
}

// issueIDToken 签发身份令牌（v3，加密载荷）。
func (g *IPGuard) issueIDToken(ip, fp string) (string, time.Time) {
	issued := time.Now()
	exp := issued.Add(idCookieLifetime)
	payload := ip + "|" + fp + "|" + strconv.FormatInt(issued.Unix(), 10) + "|" + strconv.FormatInt(exp.Unix(), 10)
	block, err := aes.NewCipher(idAEADKey(g.key))
	if err != nil {
		// 理论不可达（32B 密钥恒定）；降级 v2 保证签发不中断
		return g.issueIDTokenV2(ip, fp)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return g.issueIDTokenV2(ip, fp)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return g.issueIDTokenV2(ip, fp)
	}
	ct := aead.Seal(nil, nonce, []byte(payload), nil)
	return "v3." + base64.RawURLEncoding.EncodeToString(append(nonce, ct...)), exp
}

// issueIDTokenV2 旧版签发（仅供 v3 加密初始化失败时的降级路径）。
func (g *IPGuard) issueIDTokenV2(ip, fp string) (string, time.Time) {
	issued := time.Now()
	exp := issued.Add(idCookieLifetime)
	payload := ip + "|" + fp + "|" + strconv.FormatInt(issued.Unix(), 10) + "|" + strconv.FormatInt(exp.Unix(), 10)
	mac := hmac.New(sha256.New, g.key)
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)
	return "v2." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(sig), exp
}

// tokenStatus 令牌核验结果。区分"结构非法"（真伪造）与"签名失效"（多为服务重启 /
// IP_GUARD_SECRET 轮换后的老令牌）——后者绝不能按伪造处理，否则每次重启都会把
// 全部老访客打成 7 天封禁（旧实现的灾难路径，见 docs/ip-guard-scoring.md）。
type tokenStatus int

const (
	tokenOK        tokenStatus = iota // 有效
	tokenMalformed                    // 结构/载荷非法 → 伪造尝试
	tokenBadSig                       // 结构合法但签名不符 → 疑密钥轮换
	tokenExpired                      // 签名有效但已过期 → 正常重签
)

// verifyIDTokenEx 核验令牌并给出三态结果。v3（AEAD）优先，v2（HMAC）兼容过渡。
func (g *IPGuard) verifyIDTokenEx(token, currentIP string) (tokenStatus, string, string, time.Time) {
	parts := strings.Split(token, ".")
	if len(parts) == 2 && parts[0] == "v3" {
		return g.verifyIDTokenV3(parts[1])
	}
	if len(parts) != 3 || parts[0] != "v2" {
		return tokenMalformed, "", "", time.Time{}
	}
	payload, err1 := base64.RawURLEncoding.DecodeString(parts[1])
	sig, err2 := base64.RawURLEncoding.DecodeString(parts[2])
	if err1 != nil || err2 != nil || len(sig) != 32 {
		return tokenMalformed, "", "", time.Time{}
	}
	f := strings.Split(string(payload), "|")
	if len(f) != 4 {
		return tokenMalformed, "", "", time.Time{}
	}
	expUnix, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil {
		return tokenMalformed, "", "", time.Time{}
	}
	exp := time.Unix(expUnix, 0)
	// 先验签：签名为身份的唯一依据，签名不符时不采信任何载荷内容
	mac := hmac.New(sha256.New, g.key)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return tokenBadSig, "", "", time.Time{}
	}
	if time.Now().After(exp) {
		return tokenExpired, f[1], f[0], exp
	}
	return tokenOK, f[1], f[0], exp
}

// verifyIDTokenV3 解密并核验 v3 令牌（载荷 = ip|fp|issued|exp 的 AES-256-GCM 密文）。
func (g *IPGuard) verifyIDTokenV3(enc string) (tokenStatus, string, string, time.Time) {
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil || len(raw) < 12 {
		return tokenMalformed, "", "", time.Time{}
	}
	block, err := aes.NewCipher(idAEADKey(g.key))
	if err != nil {
		return tokenMalformed, "", "", time.Time{}
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return tokenMalformed, "", "", time.Time{}
	}
	payload, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], nil)
	if err != nil {
		// 认证失败：篡改或密钥轮换（老实例签发的 v3 密文用新密钥解不开）。
		// 映射 tokenBadSig 而非 malformed——密钥轮换绝不能按伪造封禁（见 tokenStatus 注释）
		return tokenBadSig, "", "", time.Time{}
	}
	f := strings.Split(string(payload), "|")
	if len(f) != 4 {
		return tokenMalformed, "", "", time.Time{}
	}
	expUnix, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil {
		return tokenMalformed, "", "", time.Time{}
	}
	exp := time.Unix(expUnix, 0)
	if time.Now().After(exp) {
		return tokenExpired, f[1], f[0], exp
	}
	return tokenOK, f[1], f[0], exp
}

// verifyIDToken 向后兼容包装：仅保留"有效/无效 + 绑定信息"。
func (g *IPGuard) verifyIDToken(token, currentIP string) (bool, string, string, time.Time) {
	st, fp, ip, exp := g.verifyIDTokenEx(token, currentIP)
	return st == tokenOK, fp, ip, exp
}

// ---------- 封禁执行 ----------

// isBanned 查封禁（30s 负缓存 / 到期自动视为解封）。
// 优化版：使用双重检查锁，防止 TOCTOU 竞态条件。
func (g *IPGuard) isBanned(ctx context.Context, ip string) bool {
	now := time.Now()

	// 第一次检查：快速路径
	g.mu.Lock()
	if c, ok := g.banCache[ip]; ok && now.Sub(c.at) < 30*time.Second {
		banned := c.banned && now.Before(c.expires)
		g.mu.Unlock()
		return banned
	}
	g.mu.Unlock()

	// 缓存未命中或已过期，查询数据库
	ban, err := g.store.FindBan(ctx, ip)
	banned := err == nil && ban != nil && now.Before(ban.ExpiresAt)

	expires := now
	if ban != nil {
		expires = ban.ExpiresAt
	}

	// 第二次检查：只在缓存仍未命中或已过期时更新
	g.mu.Lock()
	if c, ok := g.banCache[ip]; !ok || now.Sub(c.at) >= 30*time.Second {
		g.banCache[ip] = banCacheEntry{banned: banned, expires: expires, at: now}
	}
	g.mu.Unlock()

	return banned
}

// Whitelist 将 IP 加入临时免封禁白名单（管理端会话有效期间）。
func (g *IPGuard) Whitelist(ip string, ttl time.Duration) {
	if ip == "" {
		return
	}
	g.mu.Lock()
	g.whitelist[ip] = time.Now().Add(ttl)
	g.mu.Unlock()
}

// isWhitelisted 管理端白名单判定（顺带清理过期项）。
func (g *IPGuard) isWhitelisted(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	exp, ok := g.whitelist[ip]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(g.whitelist, ip)
		return false
	}
	return true
}

// ban 执行封禁（升级制：查旧记录 strikes+1，超 30 天衰减回初犯档）；severe 直接不低于 7 天档。
//
// 幂等保护：同一"正在进行中"的封禁期内不再升级 strikes——否则同一次违规被多类事件
// 反复触发会一路跳到 30 天（旧实现的升级放大器，会把一次误判放大成月级封禁）。
// 解封后再次违规仍按累犯正常升级。
func (g *IPGuard) ban(ctx context.Context, ip, reason string, severe bool) {
	old, err := g.store.FindBan(ctx, ip)
	if old != nil && err == nil && time.Now().Before(old.ExpiresAt) {
		log.Printf("[ipguard] 已在封禁中，不重复升级 %s（剩余 %v）：%s",
			ip, time.Until(old.ExpiresAt).Round(time.Minute), sanitizeForLog(reason))
		return
	}
	strikes := 1
	if old != nil && err == nil {
		strikes = old.Strikes + 1
		// 累犯衰减：历史封禁解封后超 30 天未再犯，按初犯处理（防止旧误封长期连坐）
		if time.Since(old.ExpiresAt) > banStrikeDecay {
			strikes = 1
		}
	}
	if severe && strikes < 3 {
		strikes = 3
	}
	dur := banDuration(strikes)
	if err := g.store.UpsertBan(ctx, ip, strikes, strikes, reason, dur); err != nil {
		log.Printf("[ipguard] 封禁写入失败 %s: %v", ip, err)
		return
	}
	g.mu.Lock()
	g.banCache[ip] = banCacheEntry{banned: true, expires: time.Now().Add(dur), at: time.Now()}
	g.mu.Unlock()
	log.Printf("[ipguard] 封禁 %s（第 %d 次，%v）：%s", ip, strikes, dur, sanitizeForLog(reason))
}

// event 记一条违规事件并做封禁决策。severe=true 仅用于**无歧义的即时严重事件**
// （流量攻击 / 结构非法的身份令牌）；其余一律走 iprisk 多证据判定，
// 避免共享出口、浏览器缩放、链接预览爬虫等场景的误封。
func (g *IPGuard) event(ctx context.Context, ip, kind, detail string, score int, severe bool) {
	if severe {
		_ = g.store.AddIPEvent(ctx, ip, kind, detail, score)
		g.ban(ctx, ip, kind+": "+detail, true)
		return
	}
	// 同类事件 5 分钟内去重（避免刷分）
	dk := ip + "|" + kind
	g.mu.Lock()
	if t, ok := g.dedupe[dk]; ok && time.Since(t) < eventDedupeTTL {
		g.mu.Unlock()
		return
	}
	g.dedupe[dk] = time.Now()
	g.mu.Unlock()
	if err := g.store.AddIPEvent(ctx, ip, kind, detail, score); err != nil {
		log.Printf("[ipguard] 事件写入失败: %v", err)
		return
	}
	g.decideBan(ctx, ip, kind)
}

// decideBan 取该 IP 近期事件做多证据决策（算法见 internal/domain/iprisk）：
// 单信号（速率/环境核验/爬虫 UA/管理端探测）在结构上无法单独致封，
// 必须"不同违规类型 ≥2 互证"或"单类型持续越线"才封。
func (g *IPGuard) decideBan(ctx context.Context, ip, trigger string) {
	events, err := g.store.ListIPEventsByIP(ctx, ip, 80)
	if err != nil {
		// 取不到明细时回退粗口径，并把门槛抬到 1.5×（宁可放过，不可误封）
		if total, err2 := g.store.RecentIPEventsScore(ctx, ip, banWindowSeconds); err2 == nil &&
			total >= int(iprisk.Threshold*1.5) {
			g.ban(ctx, ip, "累计积分 "+strconv.Itoa(total)+"（回退口径）", false)
		}
		return
	}
	evs := make([]iprisk.Event, 0, len(events))
	for _, e := range events {
		evs = append(evs, iprisk.Event{Kind: e.Kind, Score: e.Score, At: e.At})
	}
	d := iprisk.Evaluate(evs, time.Now(), g.uaDiversity(ip))
	if d.Ban {
		g.ban(ctx, ip, d.Reason, false)
		return
	}
	// 接近阈值时打观察日志，便于评估算法（不改判定）
	if d.Effective >= iprisk.Threshold*0.6 {
		log.Printf("[ipguard] 观察 %s（触发 %s）：有效分 %.0f，%d 种类型，稀释 %.2f，UA 种类 %d",
			ip, trigger, d.Effective, d.Kinds, d.Dilution, d.UADiversity)
	}
}

// uaDiversity 该 IP 窗口内不同 UA 数（共享出口识别，iprisk 稀释依据）。
func (g *IPGuard) uaDiversity(ip string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	count := 0
	if w := g.windows[ip]; w != nil {
		count = len(w.uaSet)
	}
	// 防御性默认：避免除零
	if count == 0 {
		count = 1
	}
	return count
}

// ---------- 中间件 ----------

// Middleware 全站防护：封禁 404 → 第一层速率记录 → 第三层身份核验。
func (g *IPGuard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.enabled || g.store == nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		ip := clientIPFromRequest(r)
		fp := r.Header.Get(fpHeaderName)
		if len(fp) > 128 {
			fp = "" // L-5：超长指纹按未采集处理（防 Cookie/列膨胀，不截断防绑定错乱）
		}

		// 回环/内网白名单：只记录不封禁（开发环境防自锁）
		local := g.localOK && isLocalIP(ip)

		// ① 封禁检查：全站 404。
		// 例外：/healthz（存活探针必须始终如实应答，否则外部监控把"已封禁"
		// 误报成"站点宕机"）、回环/内网、管理端白名单会话、管理端 ipguard
		// 端点（放行给后面的 adminAuth，保证被封管理员有自救解封通道，又不给
		// 攻击者任何未授权入口）。
		if g.isBanned(ctx, ip) {
			if local || g.isWhitelisted(ip) || r.URL.Path == "/healthz" ||
				r.URL.Path == "/api/v1/ip/check" ||
				strings.HasPrefix(r.URL.Path, "/api/v1/admin/ipguard/") ||
				strings.HasPrefix(r.URL.Path, "/api/v1/admin/probes") ||
				strings.HasPrefix(r.URL.Path, "/api/v1/admin/system/") {
				next.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}

		// ② 握手通道专用限流（/api/v1/site/config 免签 + 触发指纹归档写库）。
		// 走独立计数窗口：与全站速率互不干扰，也不因"通用阈值很高"被绕过。
		if !local && r.URL.Path == handshakePath {
			if !g.handshakeAllow(ctx, ip) {
				writeJSON(w, 429, map[string]any{"error": "too many requests"})
				return
			}
		}

		// ③ 第一层：滑动窗口记录（healthz 不计）
		rec := &statusWriter{ResponseWriter: w, status: 200}
		win := g.record(ip, r.UserAgent())
		path := r.URL.Path

		// ④ 第三层：身份令牌核验（API/带指纹请求）
		if !local {
			g.verifyIdentity(ctx, w, r, ip, fp)
		}

		// ⑤ 速率与爬虫判定（除白名单）
		if !local && path != "/healthz" {
			g.evaluate(ctx, ip, path, r.UserAgent(), win)
		}

		next.ServeHTTP(rec, r)

		// ⑥ 404 率统计
		if rec.status == 404 {
			g.mu.Lock()
			if w := g.windows[ip]; w != nil {
				w.notFound++
			}
			g.mu.Unlock()
		}
	})
}

// handshakeAllow 握手通道滑动窗口（60s）判定：放行 true；超限 false 并计分。
// 计数按 IP 独立存放（ipWindow.hsTimes），与全站速率窗口互不影响。
// 过滤、判定、追加必须在同一次持锁内完成：锁外化会造成并发请求基于同一快照
// 判定后互相覆盖写回（计数丢失 → 限流可被并发绕过，见安全评审 P1）。真正的
// 耗时操作（超限计分的 DB IO）在 g.event 中，保持在锁外。
func (g *IPGuard) handshakeAllow(ctx context.Context, ip string) bool {
	limit := handshakeRatePerMin()
	now := time.Now()

	g.mu.Lock()
	w := g.windows[ip]
	if w == nil {
		w = &ipWindow{uaSet: map[string]bool{}, lastFlush: now}
		g.windows[ip] = w
	}
	// 原地过滤 60s 窗口（窗口内最多 limit 条，开销可忽略）
	cutoff := now.Add(-time.Minute)
	keep := w.hsTimes[:0]
	for _, t := range w.hsTimes {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	w.hsTimes = keep
	over := len(keep) >= limit
	if !over {
		w.hsTimes = append(w.hsTimes, now)
	}
	count := len(w.hsTimes)
	g.mu.Unlock()

	if over {
		g.event(ctx, ip, "handshake-rate", sprintf("握手通道 %d req/min（上限 %d）", count, limit), 30, false)
		return false
	}
	return true
}

// handshakeRatePerMin 握手通道阈值（env HANDSHAKE_RATE_PER_MIN，默认 30）。
func handshakeRatePerMin() int {
	if v := strings.TrimSpace(os.Getenv("HANDSHAKE_RATE_PER_MIN")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return handshakeRateDefault
}

// statusWriter 捕获响应状态码。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// record 第一层记录：滑动窗口 + 小时计数 + 定期落库档案。
func (g *IPGuard) record(ip, ua string) *ipWindow {
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	if now.After(g.talkersReset) {
		g.talkers = map[string]int{}
		g.talkersReset = now.Truncate(time.Hour).Add(time.Hour)
	}
	g.talkers[ip]++
	w := g.windows[ip]
	if w == nil {
		w = &ipWindow{uaSet: map[string]bool{}, lastFlush: now}
		g.windows[ip] = w
	}
	cutoff := now.Add(-windowSize)
	keep := w.times[:0]
	for _, t := range w.times {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	w.times = append(keep, now)
	w.reqs++
	if ua != "" {
		w.uaSet[ua] = true
	}
	w.dirty = true
	// 档案落频：每 60s 一次；顺带清理闲置窗口（优化：随机采样清理，不在每次请求时全遍历）
	if now.Sub(w.lastFlush) >= profileFlushTTL {
		delta := w.reqs
		w.reqs = 0
		w.lastFlush = now
		w.dirty = false
		goSafe("ip-profile-flush", func() {
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := g.store.TouchIPProfile(c, ip, ua, delta); err != nil {
				log.Printf("[ipguard] 档案落库失败 %s: %v", ip, err)
			}
		})
	}
	// 优化：改为定期清理（在 periodicCleanup 中处理），只在超限时随机采样清理
	if len(g.windows) > 50000 && mathrand.Intn(100) == 0 {
		cutoff := now.Add(-2 * windowSize)
		cleaned := 0
		for k, x := range g.windows {
			if len(x.times) == 0 && x.lastFlush.Before(cutoff) {
				delete(g.windows, k)
				cleaned++
				if cleaned >= 100 {
					break // 每轮最多清理100条
				}
			}
		}
	}
	return w
}

// isMachineUA 脚本/机器 UA 判定（真人浏览器共享出口 vs 爬虫的分档依据）。
func isMachineUA(ua string) bool {
	if ua == "" {
		return true
	}
	low := strings.ToLower(ua)
	for _, m := range []string{"curl", "wget", "python", "scrapy", "httpclient", "go-http",
		"java/", "okhttp", "node", "axios", "postman", "httpie", "libwww"} {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
}

// evaluate 第一层/第三层违规判定（速率、爬虫、扫描器）。
// 优化版：快速拷贝数据后在锁外计算，减少锁持有时间。
func (g *IPGuard) evaluate(ctx context.Context, ip, path, ua string, w *ipWindow) {
	// 快速拷贝副本，在锁外计算
	g.mu.Lock()
	times := make([]time.Time, len(w.times))
	copy(times, w.times)
	nf := w.notFound
	g.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)
	rate1m := 0
	for _, t := range times {
		if t.After(cutoff) {
			rate1m++
		}
	}
	total := len(times)

	// 严重：流量攻击
	if rate1m >= severeRatePerMin {
		g.event(ctx, ip, "traffic-attack", sprintf("%d req/min", rate1m), 100, true)
		return
	}
	// 普通速率超限：按 UA 分档——真人浏览器（公司 NAT/飞机 WiFi 共享出口聚合
	// 大量正常用户）30 分，脚本 UA 60 分；超 3 倍阈值不分档直接 60。
	if rate1m >= warnRatePerMin {
		score := 30
		if isMachineUA(ua) || rate1m >= warnRatePerMin*3 {
			score = 60
		}
		g.event(ctx, ip, "rate", sprintf("%d req/min", rate1m), score, false)
	}
	// 管理端探测
	if strings.HasPrefix(path, "/api/v1/admin") && rate1m >= adminRatePerMin {
		g.event(ctx, ip, "admin-probe", sprintf("admin API %d req/min", rate1m), 50, false)
	}
	// 扫描器：高请求量 + 高 404 率
	if total >= scannerMinReqs && float64(nf)/float64(total) >= scanner404Ratio {
		g.event(ctx, ip, "scanner", sprintf("404 率 %.0f%%（%d/%d）", float64(nf)/float64(total)*100, nf, total), 40, false)
	}
	// 爬虫 UA
	lowUA := strings.ToLower(ua)
	if (ua == "" || strings.Contains(lowUA, "curl") || strings.Contains(lowUA, "wget") ||
		strings.Contains(lowUA, "python") || strings.Contains(lowUA, "scrapy") ||
		strings.Contains(lowUA, "httpclient") || strings.Contains(lowUA, "bot") && !strings.Contains(lowUA, "mozilla")) &&
		!strings.HasPrefix(path, "/assets/") {
		g.event(ctx, ip, "bot-ua", ua, 25, false)
	}
}

// verifyIdentity 第三层：令牌完整性 + IP↔设备绑定核验。
func (g *IPGuard) verifyIdentity(ctx context.Context, w http.ResponseWriter, r *http.Request, ip, fp string) {
	cookie, err := r.Cookie(idCookieName)
	if err != nil || cookie.Value == "" {
		// 无令牌：带指纹的 API 请求直接签发；纯页面请求等首次 API 调用再签
		if fp != "" {
			g.issue(w, r, ip, fp)
		}
		return
	}
	st, tokFp, tokIP, exp := g.verifyIDTokenEx(cookie.Value, ip)
	if st != tokenOK {
		switch st {
		case tokenMalformed:
			// 结构非法 = 明确伪造尝试（无歧义，可即时封）
			g.event(ctx, ip, "id-forgery", "身份令牌结构非法", 100, true)
		case tokenBadSig:
			// 结构合法但签名失效：绝大多数是服务重启 / IP_GUARD_SECRET 轮换后的老令牌。
			// 按低分观察处理并重签，避免"重启即封光老访客"（旧实现的灾难路径）。
			g.event(ctx, ip, "id-token-stale", "身份令牌签名失效（疑密钥轮换）", 15, false)
			g.issue(w, r, ip, fp)
		default: // tokenExpired
			g.issue(w, r, ip, fp) // 过期属正常，重新签发
		}
		return
	}
	// 令牌有效，继续后续逻辑
	// 令牌绑定的 IP 与当前不符：Cookie 被搬到别的网络。
	// 出差宽限：设备指纹与令牌绑定一致 = 同一台设备换了网络，只记录不计分；
	// 无指纹或指纹不符（Cookie 被盗搬到别的设备/网络）才按高危计分。
	if tokIP != ip {
		if fp != "" && fp == tokFp {
			g.event(ctx, ip, "id-ip-drift", sprintf("同一设备换网络 %s → %s（宽限）", tokIP, ip), 0, false)
		} else {
			g.event(ctx, ip, "id-ip-drift", sprintf("令牌绑定 %s，当前 %s", tokIP, ip), 60, false)
		}
		g.issue(w, r, ip, fp)
		return
	}
	// 设备指纹与令牌绑定的不符：Cookie 被搬到别的设备 → 高危。
	// 计分后立即按新指纹重新签发：同一次变更只留一条证据，避免被反复计分
	// （前端指纹算法升级、显示器缩放等会造成一次性指纹变化，不应累积致封）。
	if fp != "" && tokFp != "" && fp != tokFp {
		g.event(ctx, ip, "device-mismatch", "设备指纹与身份令牌绑定不符", 80, false)
		g.issue(w, r, ip, fp)
	}
	// 临近过期滑动续期
	if time.Until(exp) < 24*time.Hour {
		g.issue(w, r, ip, fp)
	}
}

// issue 签发身份令牌 Cookie。
func (g *IPGuard) issue(w http.ResponseWriter, r *http.Request, ip, fp string) {
	token, exp := g.issueIDToken(ip, fp)
	http.SetCookie(w, &http.Cookie{
		Name:     idCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode, // 允许页面导航正常回带
		// P3-1：TLS 模式下带 Secure（直连 TLS 看 r.TLS；反代终止 TLS 看 X-Forwarded-Proto，
		// 与管理端会话 Cookie 同一套口径）
		Secure:  r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		Expires: exp,
	})
}

// ---------- 对外能力（handler / 管理端用） ----------

// ReportFingerprint 第二层上报入口：登记 fp↔IP，做连坐与漂移判定。
// 返回 (响应字段, 该 IP 是否刚被封)。
func (g *IPGuard) ReportFingerprint(ctx context.Context, ip, ua, fp string, meta FingerprintMeta) (map[string]any, bool) {
	out := map[string]any{"ok": true, "banned": false}
	if !g.enabled || g.store == nil {
		return out, false
	}

	// 修复：空指纹应该记录异常事件
	if fp == "" {
		g.event(ctx, ip, "fp-report-empty", "空指纹上报", 10, false)
		out["ok"] = false
		out["error"] = "empty fingerprint"
		return out, false
	}

	local := g.localOK && isLocalIP(ip)

	// P2-2 服务端计算 MinHash 签名：原始清单（字体/扩展/插件）进签名，客户端自报的
	// 签名一律不采信——否则可伪造签名投毒 LSH 桶，把任意指纹关联到一起。
	meta.MinHashSig = g.computeMinHashSig(meta.Sets)
	meta.Sets = nil // 清单仅用于算签名，不入库

	// P2-4 时间稳定性 EWMA：需要旧档案的分量与各键稳定度，先取后写。
	old, _ := g.store.FindFingerprint(ctx, fp)
	if old != nil && len(meta.Components) > 0 {
		meta.Stability, meta.CompStability = computeStability(old.Components, old.CompStability, meta.Components)
	}

	// 上报限频：12 次/分钟
	g.mu.Lock()
	now := time.Now()
	lst := g.fpReport[ip]
	keep := lst[:0]
	for _, t := range lst {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= fpReportMaxRPM {
		g.mu.Unlock()
		out["ok"] = false
		out["error"] = "report too frequent"
		return out, false
	}
	g.fpReport[ip] = append(keep, now)
	g.mu.Unlock()

	knownIPs, err := g.store.UpsertFingerprint(ctx, fp, ip, ua, meta)
	if err != nil {
		log.Printf("[ipguard] 指纹登记失败: %v", err)
		return out, false
	}
	if local {
		return out, false
	}
	// 指纹碰撞检测：同一指纹近 10 分钟被多个不同 IP 使用 = 同型号设备指纹重合，
	// 而不是"同一台设备换网络"。此类指纹豁免连坐与漂移升级，避免无关用户被连坐。
	if g.fpColl.observe(fp, ip, time.Now()) {
		g.event(ctx, ip, "fp-collision-watch", "同一指纹 10 分钟内多 IP 并发（疑指纹碰撞，已豁免连坐）", 0, false)
		return out, false
	}
	// P2-1 感知哈希同源关联：Canvas 精确哈希会因驱动更新/抗指纹噪声漂移，
	// pHash 汉明距离近的指纹其实是同一台设备 → 记关联边（P4-4 聚类、P2-4 轮换检测的依据）。
	// 只记关联不计分（相似本身不是违规，且同型号设备有一定重合概率，交由图聚类判断）。
	g.linkByPHash(ctx, ip, fp, meta.CanvasPHash)
	// P2-2 MinHash+LSH 组件集合关联：抗"组件轮换"——字体/扩展/插件清单高度重合的
	// 指纹经 LSH 桶召回 + 精确 Jaccard > 0.8 复核后视为同源。只关联不计分，同上。
	g.linkByMinHash(ctx, ip, fp, meta.MinHashSig)
	// P2-4 换脸轮换检测：稳定分量（字体/显卡）历史稳定却在本指纹上突变，且数学指纹
	// 关联到旧指纹 → "刻意换身份"实锤。受 FP_SCORE_SHADOW 灰度控制。
	g.checkRotation(ctx, ip, fp, meta)
	// P2-5 GeoIP 交叉核验：时区↔IP 归属国跨洲矛盾、机房 ASN + 移动端 UA 组合。
	// 数据库缺失时整体降级（NewIPGuard 已处理），命中项受灰度控制。
	g.geoEnrich(ctx, ip, ua, meta)
	// P3-3 UA↔TLS 交叉核验：浏览器 UA 配上已知非浏览器 TLS 栈（curl/Go/Python 等）
	// 是伪造 header 的硬证据；指纹未知（新客户端/新版本）只记录不计分。
	// P4-1 平台证明结果由 appattest.go 的 attest 端点独立写入 attestation 列。
	g.tlsCheck(ctx, ip, ua, meta)
	// P4-5 行为生物特征：滑窗统计量入库 + 机器特征规则（完全匀速/纯直线/机械击键）。
	g.behaviorCheck(ctx, ip, meta)
	// P5-3 行为 ML 打分（模型达标才生效，shadow 纪律同规格 §0.7）。
	g.scoreBehaviorML(ctx, ip, fp, meta)
	// P4-4/P6-2 簇连坐：所在簇内有被封禁成员 → 独立弱证据（cluster-linked）。
	// 规格原文是 ×1.5，但 fp-linked 70 分已顶强类封顶——改用独立证据让 iprisk 多证互证。
	g.checkClusterCollusion(ctx, ip, fp)
	// 连坐双因子：干净设备连到被封的共享出口（酒店/机场/运营商 NAT 被前任搞封）
	// 不算违规，只记低分观察；只有"该指纹名下其他 IP 近期也有劣迹"（代理池轮换
	// 特征）才升级封禁。防误封核心：身份信号必须与设备劣迹叠加。
	bannedKin, err := g.store.BannedAmong(ctx, knownIPs)
	if err == nil && len(bannedKin) > 0 && !contains(knownIPs[:len(knownIPs)-1], ip) {
		// 只在"本 IP 新出现在该指纹下"时判定，避免已封 IP 反复上报刷日志
		if g.fpHasRecentViolations(ctx, knownIPs, ip) {
			// 指纹连坐：分数降到强类上限 70（不单独致封）。指纹碰撞（同型号设备/同镜像
			// 电脑）会造成无辜用户命中，需与 fp-churn 等第二种证据互证才封。
			g.event(ctx, ip, "fp-linked", sprintf("指纹曾关联封禁 IP %s 且设备有劣迹", strings.Join(bannedKin, ",")), 70, false)
			out["banned"] = true
			return out, true
		}
		g.event(ctx, ip, "fp-linked-watch", sprintf("指纹曾关联封禁 IP %s（干净设备宽限）", strings.Join(bannedKin, ",")), 15, false)
	}
	// 漂移：指纹全生命周期关联 IP 过多 → 代理池特征。
	// 干净设备（无劣迹）仅低分观察，有劣迹才按 40 分计。
	if len(knownIPs) > fpChurnMaxIPs {
		score := 10
		if g.fpHasRecentViolations(ctx, knownIPs, ip) {
			score = 40
		}
		g.event(ctx, ip, "fp-churn", sprintf("指纹关联 %d 个 IP", len(knownIPs)), score, false)
	}
	return out, false
}

// linkByPHash 感知哈希同源关联（P2-1）：扫描时间窗内带 pHash 的指纹，汉明距离 ≤ 10 视为同源，
// 双向边写入 fp_links（kind=phash，weight=1-distance/64）。只关联不计分：相似本身不是违规，
// 且同型号设备天然有重合概率——是否算"换脸轮换"由 P2-4 稳定性 + 图聚类判断。
func (g *IPGuard) linkByPHash(ctx context.Context, ip, fp, phash string) {
	if phash == "" || g.store == nil {
		return
	}
	if _, err := fpmath.ParsePHash(phash); err != nil {
		return // 客户端上报的哈希非法：既不关联也不计分
	}
	rows, err := g.store.ListPHashCandidates(ctx, time.Now().Add(-phashLinkWindow), phashCandidateLimit)
	if err != nil {
		log.Printf("[ipguard] pHash 候选查询失败: %v", err)
		return
	}
	cands := make([]fpmath.PHashCandidate, 0, len(rows))
	for _, r := range rows {
		if r.Fingerprint == fp {
			continue // 自身不算关联
		}
		cands = append(cands, fpmath.PHashCandidate{FP: r.Fingerprint, PHash: r.PHash})
	}
	matches := fpmath.SimilarPHashCandidates(phash, cands, fpmath.PHashDefaultMaxDistance)
	for _, m := range matches {
		weight := 1 - float64(m.Distance)/64
		if err := g.store.UpsertFPLink(ctx, fp, m.FP, "phash", weight); err != nil {
			log.Printf("[ipguard] pHash 关联写入失败: %v", err)
		}
	}
	if len(matches) > 0 {
		g.event(ctx, ip, "fp-phash-link", sprintf("感知哈希关联 %d 个旧指纹（最近距离 %d）",
			len(matches), matches[0].Distance), 0, false)
	}
}

// computeMinHashSig 由原始清单构建元素集合并计算 MinHash 签名（hex，128×16 字符）。
// 清单为空/非法 → 返回空串（该指纹不参与 MinHash 关联）。
func (g *IPGuard) computeMinHashSig(sets map[string][]string) string {
	if len(sets) == 0 {
		return ""
	}
	items := make([]string, 0, 64)
	for _, key := range []string{"fonts", "webgl_exts", "plugins"} {
		for _, v := range sets[key] {
			v = strings.TrimSpace(v)
			if v == "" || len(v) > 64 {
				continue
			}
			items = append(items, key+":"+v) // 键名进元素，避免跨清单同名值互撞
			if len(items) >= 192 {
				break
			}
		}
	}
	if len(items) == 0 {
		return ""
	}
	sig := fpmath.NewMinHash(fpmath.MinHashK).Signature(items)
	var b strings.Builder
	b.Grow(len(sig) * 16)
	buf := make([]byte, 8)
	for _, v := range sig {
		for i := 0; i < 8; i++ {
			buf[i] = byte(v >> (8 * (7 - i)))
		}
		b.WriteString(hex.EncodeToString(buf[:]))
	}
	return b.String()
}

// decodeMinHashSig hex 签名 → []uint64；长度非法返回 nil。
func decodeMinHashSig(s string) []uint64 {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) == 0 || len(b)%8 != 0 {
		return nil
	}
	out := make([]uint64, len(b)/8)
	for i := range out {
		var v uint64
		for j := 0; j < 8; j++ {
			v = v<<8 | uint64(b[i*8+j])
		}
		out[i] = v
	}
	return out
}

// linkByMinHash MinHash+LSH 组件集合关联（P2-2）：
// 1) 当前签名写 16 个 LSH 桶（先删后插，签名变化自动失效旧桶）；
// 2) 同带召回候选；3) 候选逐个取旧签名精确算 Jaccard，> 0.8 写关联边（kind=minhash）。
// 只关联不计分，理由同 pHash（P2-1）。
func (g *IPGuard) linkByMinHash(ctx context.Context, ip, fp, sigHex string) {
	if sigHex == "" || g.store == nil {
		return
	}
	sig := decodeMinHashSig(sigHex)
	if sig == nil {
		g.event(ctx, ip, "minhash-invalid", "MinHash 签名格式非法", 0, false)
		return
	}
	if len(sig) != fpmath.MinHashK {
		g.event(ctx, ip, "minhash-length", sprintf("MinHash 签名长度错误: %d != %d", len(sig), fpmath.MinHashK), 0, false)
		return
	}
	keys := fpmath.Bands(sig, fpmath.LSHBands, fpmath.LSHRows)
	bands := make([]LSHBand, 0, len(keys))
	for i, k := range keys {
		bands = append(bands, LSHBand{Band: i, Hash: k})
	}
	if err := g.store.UpsertLSHBands(ctx, fp, bands); err != nil {
		log.Printf("[ipguard] LSH 桶写入失败: %v", err)
		return
	}
	cands, err := g.store.ListLSHCandidates(ctx, fp, bands, minhashCandidateLimit)
	if err != nil {
		log.Printf("[ipguard] LSH 候选召回失败: %v", err)
		return
	}
	if len(cands) == 0 {
		return
	}
	rows, err := g.store.ListMinHashSigs(ctx, cands)
	if err != nil {
		log.Printf("[ipguard] 候选签名读取失败: %v", err)
		return
	}
	linked := 0
	best := 0.0
	for _, r := range rows {
		if r.FP == fp {
			continue
		}
		other := decodeMinHashSig(r.Sig)
		if other == nil {
			continue
		}
		j := fpmath.JaccardEstimate(sig, other)
		if j <= minhashJaccardMin {
			continue
		}
		if err := g.store.UpsertFPLink(ctx, fp, r.FP, "minhash", math.Round(j*1000)/1000); err != nil {
			log.Printf("[ipguard] MinHash 关联写入失败: %v", err)
			continue
		}
		linked++
		if j > best {
			best = j
		}
	}
	if linked > 0 {
		g.event(ctx, ip, "fp-minhash-link", sprintf("组件集合关联 %d 个旧指纹（Jaccard %.2f）", linked, best), 0, false)
	}
}

// computeStability 分量稳定度 EWMA（P2-4）：X=1 分量未变 / 0 变了，
// S = α·X + (1-α)·S_prev（α=0.3）。首次出现的分量无历史可比，跳过不写入；
// 本次未上报的旧分量保留原值。整体稳定度 = 各分量稳定度均值（0-1，四舍五入两位）。
func computeStability(oldComponents map[string]string, oldStability map[string]float64, components map[string]string) (float64, map[string]float64) {
	out := map[string]float64{}
	for k, v := range components {
		if v == "" {
			continue
		}
		if prev, existed := oldStability[k]; existed {
			out[k] = prev // 本次未上报（或新键无历史）时先继承
		}
		if oldV, ok := oldComponents[k]; !ok || oldV == "" {
			continue // 无历史值可比：不产生新观测
		}
		x := 0.0
		if oldComponents[k] == v {
			x = 1.0
		}
		out[k] = math.Round((stabilityAlpha*x+(1-stabilityAlpha)*oldStability[k])*100) / 100
	}
	sum := 0.0
	for _, v := range out {
		sum += v
	}
	overall := 0.0
	if len(out) > 0 {
		overall = math.Round(sum/float64(len(out))*100) / 100
	}
	return overall, out
}

// checkRotation 换脸轮换检测（P2-4）：pHash/MinHash 关联到的旧指纹上，某个
// 设备级稳定分量（字体清单/显卡标识）在自身生命周期里一直稳定（稳定度 ≥ 0.7），
// 却与当前指纹的取值不同 → 刻意轮换身份的实锤（正常驱动漂移只动 canvas，
// 不会连字体清单、显卡型号一起换）。命中记 env-flag:fpb_rotation_detected，
// 计分受 FP_SCORE_SHADOW 灰度控制（规格 §0.7）。
func (g *IPGuard) checkRotation(ctx context.Context, ip, fp string, meta FingerprintMeta) {
	if g.store == nil || len(meta.Components) == 0 {
		return
	}
	links, err := g.store.ListFPLinks(ctx, fp, 10)
	if err != nil || len(links) == 0 {
		return
	}
	now := time.Now()
	flips := []string{}
	var linkedOld []string
	for _, l := range links {
		other := l.Dst
		if other == fp {
			other = l.Src
		}
		if other == fp || other == "" {
			continue
		}
		tgt, err := g.store.FindFingerprint(ctx, other)
		if err != nil || tgt == nil {
			continue
		}
		if now.Sub(tgt.LastSeen) < rotationOldMinAge {
			continue // 目标不够"旧"（可能是同报文内的回环），不构成换脸
		}
		linkedOld = append(linkedOld, other[:min(8, len(other))])
		for _, k := range rotationWatchKeys {
			cur := meta.Components[k]
			old := tgt.Components[k]
			if cur == "" || old == "" || old == cur {
				continue
			}
			if tgt.CompStability[k] >= stabilityStableMin {
				flips = append(flips, k)
			}
		}
	}
	if len(flips) == 0 {
		return
	}
	score := 0
	if !shadowScoring() {
		score = rotationDetectedScore
	}
	g.event(ctx, ip, "env-flag:fpb_rotation_detected",
		sprintf("换脸轮换：稳定分量 %s 突变且关联旧指纹 %s", strings.Join(uniq(flips), ","), strings.Join(uniq(linkedOld), ",")), score, false)
}

// EntropyFactor 封禁触发系数（P2-3）：min(1, entropy_bits/40)。
// 熵权未计算（0）或档案缺失 → 1（不放大也不衰减，行为与 P2 之前一致）。
func (g *IPGuard) EntropyFactor(ctx context.Context, fp string) float64 {
	if g.store == nil || fp == "" {
		return 1
	}
	row, err := g.store.FindFingerprint(ctx, fp)
	if err != nil || row == nil || row.EntropyBits <= 0 {
		return 1
	}
	return math.Min(1, row.EntropyBits/entropyFactorBits)
}

// SetJA4DB 注入 JA4 → 应用名 映射库（cli 装配时从 data/ja4-mapping.csv 加载）。
// 未注入或未加载 → UA↔TLS 交叉核验整体降级跳过。
func (g *IPGuard) SetJA4DB(db JA4Mapper) { g.ja4 = db }

// tlsCheck UA↔TLS 交叉核验（P3-3，L1×L2 交汇点）：
//   - JA4 ∈ 已知非浏览器栈 且 UA 声称浏览器 → env-flag:fpb_ua_tls_mismatch
//     （高置信 +25，灰度期 0 分只记录）；
//   - JA4 不在已知库 → env-flag:tls_unknown 仅记录（新版本浏览器/未知工具，不计分）；
//   - 映射库未加载 / 纯 HTTP 部署（JA4 为空）→ 静默跳过。
func (g *IPGuard) tlsCheck(ctx context.Context, ip, ua string, meta FingerprintMeta) {
	if g.store == nil || g.ja4 == nil || !g.ja4.Loaded() || meta.JA4 == "" {
		return
	}
	app, ok := g.ja4.Lookup(meta.JA4)
	if !ok {
		g.event(ctx, ip, "env-flag:tls_unknown",
			sprintf("TLS 指纹 %s 不在已知库（新客户端或未知工具）", meta.JA4), 0, false)
		return
	}
	if ja4db.IsNonBrowserApp(app) && uaClaimsBrowser(ua) {
		score := 0
		if !shadowScoring() {
			score = uaTLSMismatchScore
		}
		g.event(ctx, ip, "env-flag:fpb_ua_tls_mismatch",
			sprintf("TLS 栈为 %s（%s），UA 却声称浏览器", app, meta.JA4), score, false)
	}
}

// uaClaimsBrowser UA 是否声称主流浏览器（Chrome/Edge/Firefox/Safari 系）。
func uaClaimsBrowser(ua string) bool {
	l := strings.ToLower(ua)
	return strings.Contains(l, "chrome") || strings.Contains(l, "firefox") ||
		strings.Contains(l, "safari") || strings.Contains(l, "edg/")
}

// geoEnrich GeoIP 交叉核验（P2-5）：补全 ip_profiles 的 ASN/国家，并做两项检测——
//   - 时区大洲 ↔ IP 归属国跨洲不符 → env-flag:fpb_tz_geo_mismatch（+10，灰度 0 分）
//   - 机房 ASN + 移动端 UA 组合 → env-flag:fpb_hosting_mobile_ua（+15，灰度 0 分）
//
// 数据库缺失/解析失败一律静默跳过（离线部署纪律）。
func (g *IPGuard) geoEnrich(ctx context.Context, ip, ua string, meta FingerprintMeta) {
	if g.store == nil || g.geo == nil || !g.geo.Enabled() {
		return
	}
	pip := net.ParseIP(ip)
	if pip == nil || pip.IsPrivate() || pip.IsLoopback() {
		return
	}
	country := g.geo.Country(pip)
	asn, org := g.geo.ASN(pip)
	asnType := ""
	if geoip.HostingOrg(org) {
		asnType = "hosting"
	} else if org != "" {
		asnType = "isp"
	}
	_ = g.store.UpdateIPGeo(ctx, ip, asn, asnType, country, meta.TZ)
	if asnType == "hosting" && isMobileUA(ua) {
		score := 0
		if !shadowScoring() {
			score = geoHostingMobileUAScore
		}
		g.event(ctx, ip, "env-flag:fpb_hosting_mobile_ua",
			sprintf("机房出口（AS%d %s）+ 移动端 UA", asn, org), score, false)
	}
	if meta.TZ != "" && geoip.ContinentMismatch(country, meta.TZ) {
		score := 0
		if !shadowScoring() {
			score = geoTZMismatchScore
		}
		g.event(ctx, ip, "env-flag:fpb_tz_geo_mismatch",
			sprintf("客户端时区 %s 与 IP 归属国 %s 跨洲不符", meta.TZ, country), score, false)
	}
}

// isMobileUA UA 是否声称移动端（Android/iOS 设备）。
func isMobileUA(ua string) bool {
	l := strings.ToLower(ua)
	return strings.Contains(l, "android") || strings.Contains(l, "iphone") ||
		strings.Contains(l, "ipad") || strings.Contains(l, "mobile")
}

// RefreshEntropyBits 每日 cron 任务（P2-3）：对近 30 天指纹按分量值出现频率重算
// 熵权并写回 entropy_bits。大众配置 bits 低、罕见组合 bits 高，封禁触发系数
// min(1, bits/40) 由 EntropyFactor 在三层积分时套用。
func (g *IPGuard) RefreshEntropyBits(ctx context.Context) error {
	if g.store == nil {
		return nil
	}
	rows, err := g.store.ListFingerprintsSince(ctx, time.Now().Add(-entropyWindow), entropyMaxRows)
	if err != nil {
		return err
	}
	if len(rows) < 2 {
		return nil // 样本不足：单条指纹任何值都是"唯一"，熵权无意义
	}
	sets := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		sets = append(sets, r.Components)
	}
	bits := fpmath.EntropyWeights(sets)
	sum := 0.0
	for i := range rows {
		if err := g.store.UpdateEntropyBits(ctx, rows[i].Fingerprint, bits[i]); err != nil {
			log.Printf("[ipguard] 熵权写入失败 %s: %v", rows[i].Fingerprint, err)
			continue
		}
		sum += bits[i]
	}
	log.Printf("[ipguard] 熵权刷新完成：%d 条指纹，均值 %.1f bit", len(rows), sum/float64(len(rows)))
	return nil
}

// RefreshClusters 图聚类每日任务（P4-4）：30 天窗口内按四类边构连通分量，
// 全量重写 fp_clusters 与 cluster_id。簇 ≥ 3 供管理端 review 关注（连坐系数
// ×1.5 的接入留待与 iprisk 单类封顶设计对齐后启用，见交付笔记待办）。
func (g *IPGuard) RefreshClusters(ctx context.Context) error {
	if g.store == nil {
		return nil
	}
	since := time.Now().Add(-30 * 24 * time.Hour)
	rows, err := g.store.ListFingerprintsSince(ctx, since, entropyMaxRows)
	if err != nil {
		return err
	}
	links, err := g.store.ListAllFPLinks(ctx, since, 200000)
	if err != nil {
		return err
	}
	nodes := make([]fpcluster.NodeInput, 0, len(rows))
	for _, r := range rows {
		nodes = append(nodes, fpcluster.NodeInput{
			FP: r.Fingerprint, IPs: r.IPs, UA: r.UA, JA4: r.JA4,
			BehaviorJSON: r.BehaviorJSON, ClockSkew: r.ClockSkewPPM,
		})
	}
	linksDTO := make([]fpcluster.LinkInput, 0, len(links))
	for _, l := range links {
		linksDTO = append(linksDTO, fpcluster.LinkInput{Src: l.Src, Dst: l.Dst, Kind: l.Kind})
	}
	clusters := fpcluster.Build(nodes, linksDTO)
	dtos := make([]ClusterDTO, 0, len(clusters))
	maxSize := 0
	for _, c := range clusters {
		if c.Size > maxSize {
			maxSize = c.Size
		}
		dtos = append(dtos, ClusterDTO{Members: c.Members, Size: c.Size, FirstSeen: time.Now(), Reason: c.Reason})
	}
	if err := g.store.ReplaceClusters(ctx, dtos); err != nil {
		return err
	}
	log.Printf("[ipguard] 图聚类：%d 指纹 / %d 簇（最大 %d 成员）", len(rows), len(clusters), maxSize)
	return nil
}

// behaviorCheck 行为生物特征（P4-5）：清洗入库（meta.Behavior 已裁剪），机器特征规则
// 命中记 env-flag:behavior_machine（+15，灰度 0 分）。无数据（headless 无鼠标事件）
// 不等于机器——规则只在事件数达标时生效。
func (g *IPGuard) behaviorCheck(ctx context.Context, ip string, meta FingerprintMeta) {
	if g.store == nil || meta.Behavior == "" {
		return
	}
	f := behavior.Parse(meta.Behavior)
	if f == nil {
		return
	}
	signals := behavior.MachineSignals(f)
	if len(signals) == 0 {
		return
	}
	score := 0
	if !shadowScoring() {
		score = behaviorMachineScore
	}
	g.event(ctx, ip, "env-flag:behavior_machine",
		sprintf("行为机器特征 %s（鼠标 %d 事件 / 击键 %d 事件）", strings.Join(signals, ","), f.Mouse.Events, f.Keys.Events), score, false)
}

// checkClusterCollusion 簇连坐（P4-4/P6-2）：fp 所在簇内有被封禁成员 →
// 独立弱证据 cluster-linked 15 分（灰度 0 分）。规格原文 ×1.5 会在 fp-linked 70 分
// 上越强类封顶——改用独立证据让 iprisk 多证互证（安全边界与 P2-1 一致）。
func (g *IPGuard) checkClusterCollusion(ctx context.Context, ip, fp string) {
	if g.store == nil {
		return
	}
	row, err := g.store.FindFingerprint(ctx, fp)
	if err != nil || row == nil || row.ClusterID == nil {
		return
	}
	clusters, err := g.store.ListClusters(ctx, 100)
	if err != nil {
		return
	}
	for _, c := range clusters {
		if c.ID != *row.ClusterID {
			continue
		}
		for _, m := range c.Members {
			if m == fp {
				continue
			}
			if b, _ := g.store.FindBan(ctx, firstIP(g.ipsOf(ctx, m))); b != nil {
				g.event(ctx, ip, "cluster-linked",
					sprintf("簇 %d 成员 %s 已被封禁", c.ID, m[:min(8, len(m))]), 15, false)
				return
			}
		}
	}
}

// fpHasRecentViolations 抽查指纹名下其他 IP 近期（7 天）是否有违规记录：// 区分"出差换网络的干净设备"与"代理池轮换的指纹"。
func (g *IPGuard) fpHasRecentViolations(ctx context.Context, knownIPs []string, exceptIP string) bool {
	probed := 0
	for i := len(knownIPs) - 2; i >= 0 && probed < fpViolationProbe; i-- {
		other := knownIPs[i]
		if other == exceptIP {
			continue
		}
		probed++
		if score, err := g.store.RecentIPEventsScore(ctx, other, fpViolationLookback); err == nil && score > 0 {
			return true
		}
	}
	return false
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// uniq 去重保序（轮换检测的分量名/关联指纹展示用）。
func uniq(xs []string) []string {
	seen := map[string]bool{}
	out := xs[:0]
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// Talkers Top 访客（内存近 1 小时计数）。
func (g *IPGuard) Talkers() []map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	type kv struct {
		ip string
		n  int
	}
	all := []kv{}
	for ip, n := range g.talkers {
		all = append(all, kv{ip, n})
	}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].n > all[i].n {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	out := []map[string]any{}
	for i, x := range all {
		if i >= 10 {
			break
		}
		out = append(out, map[string]any{"ip": x.ip, "reqs": x.n})
	}
	return out
}

// ResetBanCache 解封后刷新缓存。
func (g *IPGuard) ResetBanCache(ip string) {
	g.mu.Lock()
	delete(g.banCache, ip)
	g.mu.Unlock()
}

// BanIP 手动封禁（管理端）。
func (g *IPGuard) BanIP(ctx context.Context, ip, reason string, hours int) error {
	dur := time.Duration(hours) * time.Hour
	if dur <= 0 {
		dur = 24 * time.Hour
	}
	old, err := g.store.FindBan(ctx, ip)
	strikes := 1
	if old != nil && err == nil {
		strikes = old.Strikes + 1
	}
	if err := g.store.UpsertBan(ctx, ip, strikes, strikes, reason, dur); err != nil {
		return err
	}
	g.mu.Lock()
	g.banCache[ip] = banCacheEntry{banned: true, expires: time.Now().Add(dur), at: time.Now()}
	g.mu.Unlock()
	log.Printf("[ipguard] 手动封禁 %s（%v）：%s", ip, dur, sanitizeForLog(reason))
	return nil
}

// MLModelHealthStatus 暴露ML模型加载器健康状态（用于§10诊断页）。
func (g *IPGuard) MLModelHealthStatus() map[string]interface{} {
	if g.mlEngine == nil {
		return map[string]interface{}{"enabled": false}
	}
	return g.mlEngine.HealthStatus()
}
