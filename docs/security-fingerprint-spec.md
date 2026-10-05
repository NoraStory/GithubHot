# GithubHot 反伪造指纹与防护体系 — 实施规格书

> 交付对象：Coding Agent（自动化执行）。本文档是唯一需求来源，执行时不需要额外上下文。
> 覆盖范围：仅安全/指纹体系。不涉及业务功能（榜单、日报、信源）。
> 阶段划分：P0 硬伤修复 → P1 指纹浏览器识别 → P2 算法升级 → P3 TLS+JA4 → P4 平台证明与高级项 → P5 行为机器学习 → P6 图算法升级（Louvain / GNN / MIDAS）。

---

## 0. 执行须知（必读红线）

1. **命令环境**：PowerShell 7.x。前端命令在 `web/` 内执行；Go 命令在仓库根执行。
2. **依赖安装位置**：npm 依赖只装进 `web/package.json`（`cd web; npm install <pkg>`）；Go 依赖进 `go.mod`（`go get`）。禁止全局安装。
3. **提交纪律**：仓库有 `.git`。每完成一个阶段（P0/P1/...）并通过该阶段验收后提交一次；提交信息中文、带 scope 前缀（如 `security: P1 指纹浏览器六路检测采集`）。禁止提交 `.env`、密钥、`data/*.db`。
4. **每阶段完成门槛**：`go vet ./...` 与 `go test ./...` 全绿；`cd web; npm run build` 成功。任一失败不得提交。
5. **架构纪律（DDD）**：纯算法（pHash、MinHash、熵、回归）放 `internal/domain` 新包，零 IO；仓储与外部调用放 `internal/infrastructure`；HTTP 端点放 `internal/interfaces/httpapi`。依赖方向严格 `interfaces/infrastructure → application → domain`。
6. **向后兼容**：APP（Android）与旧网页客户端同时在跑。所有 `/api/v1/fp/report` 新增字段必须可选；旧客户端不上报新字段时行为与现状完全一致。
7. **灰度纪律（防假阳性）**：每个新增检测 flag 先以"仅记录不计分"模式上线（env 开关 `FP_SCORE_SHADOW=1` 默认开），管理端可见命中统计；累计 7 天且假阳性率 < 0.5% 后才允许接入违规积分。规格中所有"计分"步骤都必须尊重此开关。
8. **License 红线**：
   - JA4（TLS 客户端指纹算法本体）BSD-3，可自由使用。
   - JA4S/H/L/X/SSH/T 等"+"系列为 FoxIO License 1.1：本项目（开源、非售卖）内部使用允许，但**禁止**将其实现代码用于商业产品转售，且需在 `NOTICE` 或 README 致谢标注。
   - CreepJS 的检测思路可参考，**禁止**复制其代码（license 不明确）。
   - 每引入一个第三方库，落地时核对仓库 LICENSE 文件并在本文档 §11 表格中回填实际 license。

---

## 1. 现状基线（勘察结论，file:line 为准）

### 1.1 前端采集（web/src/lib/fingerprint.js）

| 信号 | 位置 | 说明 |
|---|---|---|
| Canvas 渲染 | :21 | 精确 SHA-256，驱动更新即漂移 |
| WebGL UNMASKED 厂商/显卡/扩展/精度 | :42 | 声明型，可被伪造 |
| 音频栈 | :64 | AudioContext |
| 字体枚举 | :84 | 列表型 |
| WebRTC STUN srflx 真实 IP | :145 | |
| 环境信号（UA/platform/语言/时区/屏幕/DPR/CPU/内存/触点） | :170 | |
| fp = SHA256(分量融合) | :188 | **主键，全有全无** |
| envAudit 客户端自检 | :107 | flags 客户端生成，服务端全盘信任 |
| POST /api/v1/fp/report | :191 | |

### 1.2 服务端核验现状

- `internal/interfaces/httpapi/ipguard_api.go:76-91`：Sec-CH-UA-Platform ↔ UA 交叉（唯一核验）。
- `:93`：UA 直查 headless 关键词。
- `internal/interfaces/httpapi/ipguard.go:561-600`：`gh_id` 令牌 HMAC 与 IP/指纹绑定比对。
- **无** UA↔TLS 比对、**无** IP 地理 vs 时区核验、客户端 flags 不做服务端复核。

### 1.3 IP 防护三层（ipguard.go:19-40 总览）

- 一层（:449,:514）：内存滑动窗口（5min 速率/404 率/UA 集合）；≥600/min 直封、≥150/min 分档 30/60 分；60s 落库 `ip_profiles`。
- 二层（:620-677）：指纹上报 12 次/分限频；BannedAmong 连坐 + churn（>12 IP）双因子。
- 三层（:236,:349-376）：`gh_id` HMAC Cookie 绑 `ip|fp|exp`；10min 积分 ≥100 封禁；档位 30m/24h/7d/30d（:135）；全部内存 map，重启丢失。

### 1.4 AppGuard（appguard.go）

- `X-App-Sign` HMAC 签名，nonce 内存去重（:194）。
- **硬伤 A**：握手通道 `/api/v1/site/config` 免签（:126）。
- **硬伤 B**：seed 默认 `gh-dev-seed-v1` 且明文随 site/config 下发（`spa.go:303,311`）→ 签名机制对任何人可复现。
- `X-Browser-Fp` 键白名单 canvas/webgl/audio/fonts/renderer/screen（:273）。

### 1.5 Android（android-app/，独立仓库 GithubHot-App）

- `DeviceFingerprint.kt:22-33`：ANDROID_ID + Build 字段 → SHA-256[:32]（root/adb 可改）。
- `AppIntegrity.kt:28-33`：APK 签名客户端自检（比对 CI 注入期望值 + 内存 canary），**结果不上报服务端**。
- `ApiClient.kt:50-71`：每请求 8 头。
- **无** Play Integrity / SafetyNet。

### 1.6 HTTP 栈与会话

- chi v5 + net/http（server.go:36）；`serve.go:45-46` 纯 `ListenAndServe`，**无 TLS、无 HTTP/2**、无自定义 Listener（无 ClientHello 拦截挂载点）。
- 管理端：Argon2id 64MiB/t=3/p=2（adminauth.go:17）；`gh_admin_session` HttpOnly+Strict（auth.go:227）；`admin_sessions` 表存 ip（db.go:96）但 `checkAdminSession` **丢弃 ip 不校验**（auth.go:73）；爆破 5 次 severe 封禁（auth.go:199）。
- `gh_id` cookie `Secure=false`（ipguard.go:603）。

---

## 2. 分层防御模型（目标态）

```
L3 平台证明层   Play Integrity（Android）/ WebAuthn 通行密钥 / mTLS   ← 密码学不可伪造
L2 服务端核验层 UA↔JA4 · 时区↔IP地理 · ASN/机房 · 熵权 · 稳定性      ← 组合一致性
L1 协议指纹层   JA4（TLS）· H2 SETTINGS · JA4H 头序                  ← 重组网络栈才能改
L0 客户端自报层 Canvas(pHash) · WebGL · 字体 · WebRTC · 行为生物特征  ← 现有+升级，交叉核验兜底
```

攻击者成本模型：伪造 L0 需反检测浏览器（camoufox 12.3k★ 级）；过 L1 需 curl_cffi/wreq 级整栈模拟；过 L2 需全层一致；过 L3 需攻破 Google/硬件安全芯片。**六路全过成本远超刷站收益，即防御胜利条件。**

---

## 3. P0 — 现有硬伤修复（预计半天，最高优先级）

### P0-1 AppGuard seed 停止下发 + 强制非默认

- **改法**：
  1. `spa.go:303,311`：从 `/api/v1/site/config` 响应中**删除** seed 字段。
  2. seed 改为仅从 env `APP_SIGN_SEED` 读取；`config/config.go` 启动校验：未设置或等于 `gh-dev-seed-v1` 时，serve 模式直接 `log.Fatal`（run/mcp 模式警告降级）。
  3. `.env.example` 增加 `APP_SIGN_SEED=` 及生成说明（`githubhot admin seed` 新子命令：输出 32 字节 base64 随机值，供用户粘贴进 .env）。
- **兼容性**：存量 APP 从 site/config 拿 seed 的链路断掉 → APP 端需同步发版把 seed 改为构建期注入（Gradle `buildConfigField`，CI Secret `APP_SIGN_SEED` 注入）。若 APP 发版滞后，服务端可设 `APP_SIGN_SEED_GRACE=旧值1,旧值2` 过渡期双 seed 验签，过渡期默认 14 天。
- **验收**：抓包 `/api/v1/site/config` 无 seed 字段；不配置 seed 时 serve 拒绝启动；配置新 seed 后 APP（注入同值）签名通过。

### P0-2 管理会话绑定 IP

- **改法**：`auth.go:73` `checkAdminSession` 增加校验：会话记录 IP 与当前请求 IP 不一致 → 拒绝并记 severe 违规（复用 auth.go:199 封禁通道）。考虑到移动网络出口变化，默认放宽为 **/24 前缀一致**（IPv4）或 /64（IPv6）；env `ADMIN_SESSION_IP_STRICT=1` 时完全一致才过。
- **验收**：单测：同 IP 过、同 /24 过、跨 /24 拒绝并计分。

### P0-3 握手通道纳入限流

- **改法**：`appguard.go:126` 的 `/api/v1/site/config` 免签白名单收窄——仍然免签（APP 冷启动需要），但挂一层滑动窗口限流（复用 ipguard.go:449 现有实现，阈值 env `HANDSHAKE_RATE_PER_MIN` 默认 30/min）。
- **验收**：压测 60 req/min 命中限流响应 429 并计分。

### P0-4 面板可见性（为 P1/P2 铺路）

- `web/src/admin/` 的 IP 防护面板增加"检测 flag 命中统计"区块：读 `ip_fingerprints.flags` JSON 里的各 key 出现次数聚合（SQL json_each 或应用层聚合）。P1/P2 的灰度观察全靠它。

---

## 4. P1 — 指纹浏览器识别（前端六路采集 + BotD，1-2 天）

全部改动落在 `web/src/lib/fingerprint.js`（新增检测模块）与 `ipguard_api.go`（服务端计分）。新增 flags 通过现有 `/api/v1/fp/report` 的 flags JSON 上报，**表结构零改动**。

### P1-1 干净环境对照（杀 JS 层 patch）

**Worker-Canvas 对照**：

```
1. 主线程：canvas.toDataURL(hashInstr(A)) → hashMain
2. Worker：new OffscreenCanvas(同尺寸).getContext('2d') 执行 hashInstr(A)
   → convertToBlob → arrayBuffer → SHA-256 → hashWorker
3. hashInstr(A)：固定矢量图形+渐变+多字号文字组合（避免字体加载竞态：只用系统默认字体）
4. flag: fpb_canvas_diverge = (hashMain !== hashWorker)
```

注意：Worker 脚本以 blob URL 内联（`new Worker(URL.createObjectURL(new Blob([...])))`），不新增静态文件。浏览器不支持 OffscreenCanvas 时跳过并记 `fpb_canvas_check_unsupported`（不计分）。

**iframe 参照 diff**：运行时创建同源 iframe（about:blank），对 `navigator.webdriver / navigator.plugins.length / navigator.hardwareConcurrency / screen.colorDepth` 取 `contentWindow` 参照值与主框架比对，任一不等 → `fpb_iframe_diverge`。

**音频双通道**：`AudioContext` 与 `OfflineAudioContext` 各渲染同一 chirp 信号，输出 buffer 的 SHA-256 不等 → `fpb_audio_diverge`。

### P1-2 能力—声明核验（对内核级 patch 也有效）

- **WebGL 能力集**：采集 `MAX_TEXTURE_SIZE / MAX_VIEWPORT_DIMS / 压缩纹理格式清单 / 扩展清单 / 着色器精度`，与 `UNMASKED_RENDERER_WEBGL` 声明的 GPU 型号做查表核验。数据表：`web/src/lib/gpu-capabilities.json`（首版收录 top 20 桌面 GPU 的能力集，来源：手工采集 + CreepJS 公开数据整理；落地时注意去重整理为自有格式）。核验失败 → `fpb_gpu_claim_mismatch`。
- **字体实测**：对枚举出的字体清单抽样 10 个，`measureText` 宽度与 fallback 渲染宽度一致 → 该字体是谎报，≥3 个谎报 → `fpb_font_claim_mismatch`。
- **核数基准**：`hardwareConcurrency` 声明值 vs 固定微基准（1e7 次整数运算耗时落档）严重不符（声明的核数与耗时不在同一性能档）→ `fpb_cores_claim_mismatch`（只记录，阈值宽，防 CPU 节流假阳性）。

### P1-3 JS 拦截取证（杀 JS 层 patch）

**getter 时序侧信道**（核心算法，务必按伪代码实现）：

```
target 属性集: navigator.webdriver, navigator.plugins, screen.colorDepth, ...
baseline: Object.getOwnPropertyDescriptor(Object.prototype, 'toString') 的读取（已知未被 patch）
for each target:
    N = 100_000
    t0 = performance.now(); for(i<N) void obj.prop; t1 = performance.now()
    p50_target = (t1-t0)/N
同法测 baseline 得 p50_base
判定: p50_target > max(1µs, 20 × p50_base) → 拦截嫌疑
```

- 浏览器计时器被 Spectre 缓解削精度（~100µs 粗化），**必须用 N 次循环摊薄取均值**，且每属性测 3 轮取中位数。
- 输出 `fpb_getter_timing` = 嫌疑属性清单（数组），不输出原始时序（省流量）。
- **只记录不计分**（噪声大，灰度观察 7 天后按 §0.7 决定是否计分）。

**native function 取证**：对 `navigator.permissions.query`、`HTMLCanvasElement.prototype.toDataURL` 等热点函数检查 `Function.prototype.toString.call(fn)` 是否含 `[native code]`、`fn.name`/`fn.length` 与已知原生签名一致、`fn.toString.toString()` 结果正常。异常 → `fpb_native_fn_tamper`。

**Error.stack 版本指纹**：`new Error().stack` 的格式（V8 版本相关特征：`at fn (file:line:col)` 格式细节）与 UA 声称的 Chrome 大版本交叉，明显跨代 → `fpb_stack_version_mismatch`。

### P1-4 引入 BotD（现成弹药）

- `cd web; npm install @fingerprintjs/botd`（MIT，纯客户端）。
- `fingerprint.js` 加载后调用 `BotD.load().then(b => b.detect())`，结果展开为 `botd_<signal>` flags（如 `botd_webdriver_1`）。
- **验收**：playwright headless 访问本地站点，flags 至少命中 `botd_headless`；正常 Chrome 手动访问零命中。

### P1-5 服务端计分接入

- `ipguard_api.go` 处理 fp/report 时解析新 flags：`fpb_canvas_diverge / fpb_gpu_claim_mismatch / fpb_iframe_diverge / fpb_audio_diverge / fpb_native_fn_tamper` 每项 +15 分（走三层积分 :349-376 现有通道）；`fpb_getter_timing / fpb_stack_version_mismatch / fpb_cores_claim_mismatch` 仅记录（`FP_SCORE_SHADOW` 控制）。
- 积分阈值/档位复用现有 ：135 配置，不新增档位。

### P1-6 验收清单

1. 正常 Chrome/Edge/Safari（桌面+安卓各一）访问 7 天，`fpb_*` 假阳性 < 0.5%（看 P0-4 面板统计）。
2. `npx patchright`（undetected playwright）脚本访问：至少命中 2 个 `fpb_*` 或 `botd_*`。
3. 若本机装了 camoufox：camoufox 访问 → ①②类 flag 允许不命中，但 `fpb_gpu_claim_mismatch` 或物理层（P4 时钟偏移）至少一项命中。无 camoufox 环境则跳过此项并在交付说明中注明。

---

## 5. P2 — 算法升级（2-3 天）

### P2-1 pHash 替代 Canvas 精确哈希（抗指纹漂移）

- **算法**（前端实现，纯函数放 `web/src/lib/phash.js`，可 `node --test` 单测）：
  1. canvas（P1-1 的 hashInstr 输出）→ `getImageData` → 灰度化（0.299R+0.587G+0.114B）→ 双线性降采样 32×32；
  2. DCT-II 变换（32×32）；取左上 8×8 低频块（跳过 [0][0] DC 项）；
  3. 以 64 系数的中位数为阈值二值化 → 64bit pHash，hex16 输出。
- **数据模型**：`ip_fingerprints` 新增列 `canvas_phash TEXT`（迁移：modernc sqlite 支持 `ALTER TABLE ADD COLUMN`，参照现有迁移写法）。
- **关联查询**：新指纹入库时 `SELECT fp FROM ip_fingerprints WHERE canvas_phash != '' ` 近 30 天记录，汉明距离 ≤ 10 视为同源（Go 侧 popcount；数据量 <1e5 时全扫可接受，>1e5 再考虑 BK-tree）。
- **fp 主键不变**：`fp = SHA256(融合)` 仍做精确身份；pHash 只做"相似关联"，命中时在 `ip_fingerprints` 关联表记 `linked_phash_fp`（后续图聚类用）。
- **单测**：同一指令渲染两次 pHash 相同；叠加 1% 像素噪声后汉明距离 < 10；完全不同图形距离 > 30。

### P2-2 MinHash + LSH 分桶（抗组件轮换）

- **位置**：纯算法 `internal/domain/fpmath/minhash.go`（零 IO）；桶存储 `internal/infrastructure/sqlite/lsh.go`。
- **算法**：
  - 输入集合：字体清单、WebGL 扩展清单、已安装插件（来自现有 components）。
  - MinHash 签名：k=128 个哈希函数 `h_i(x) = (a_i·x + b_i) mod p`（p=2^61-1，a/b 随机固定种子），签名 = 每函数下集合元素的最小哈希，存 hex（128×32bit）。
  - LSH：b=16 带 × r=8 行；某带内 8 个 minhash 全等 → 落同桶。
  - 新指纹入库：查桶 → 桶内候选再精确算 Jaccard，> 0.8 → 关联。
- **DDL**：`CREATE TABLE fp_lsh_buckets (band INTEGER, bucket_hash TEXT, fp TEXT, created_at TEXT)`；`(band, bucket_hash)` 建索引。
- **存储**：`ip_fingerprints` 新增 `minhash_sig TEXT`。
- **单测**：20 个组件改 1 个仍同桶；完全不同集合不同桶；Jaccard 估计误差 < 0.1。

### P2-3 熵值加权（信号定权）

- **算法**：对每个分量值统计 30 天历史出现次数 `c(v)`，权重 `w = -log2(c(v)/N)`；指纹整体置信度 `bits = Σ w(命中的分量)`。
- **位置**：`internal/domain/fpmath/entropy.go` + 每日定时任务（复用现有 cron）刷新 `ip_fingerprints.entropy_bits`。
- **用途**：高熵指纹（罕见组合）可信度高 → 违规时封禁更果断；低熵（大众配置）→ 只计分不硬封。接入三层积分：违规分 × `min(1, entropy_bits/40)` 作为封禁触发系数。

### P2-4 时间稳定性评分

- EWMA：`S_t = α·X_t + (1-α)·S_{t-1}`，α=0.3；X 为分量是否变化（0/1）。
- 每指纹每分量维护稳定度，整体 `stability` 列（REAL，0-1）。
- 稳定分量（字体集、WebGL 扩展）突变 + pHash/MinHash 关联到旧指纹 → "换脸"实锤 → `fpb_rotation_detected` 计 +25 分。

### P2-5 GeoIP 交叉核验（时区/语言 ↔ IP 地理）

- **数据源**：[ip-location-db](https://github.com/sapics/ip-location-db) 的 `geo-whois-asn-country.mmdb`（CC BY 4.0，直链可下，免账号；README 数据致谢必须加）。首次 serve 启动时若 `data/geo.mmdb` 不存在 → 打印下载 URL 提示，不自动下载（尊重离线部署）；提供 `githubhot geo download` 子命令用 safehttp 下载。
- **读取**：`go get github.com/oschwald/geoip2-golang`。
- **核验**：
  1. 客户端时区偏移（fingerprint.js :170 已采）vs `geo_tz` 不符 → `fpb_tz_geo_mismatch`（跨洲级不符才计分，+10；邻区忽略）；
  2. `Accept-Language` 主语言 vs 国家不符 → 仅记录；
  3. ASN 类型：ip-location-db 的 asn 库判断 hosting（数据中心）→ `ip_profiles.asn_type='hosting'`，配合 UA 声称移动端 → `fpb_hosting_mobile_ua` +15。
- **DDL**：`ip_profiles` 新增 `asn INTEGER, asn_type TEXT, geo_country TEXT, geo_tz TEXT`。
- **env**：`GEOIP_DB_PATH`（默认 `data/geo.mmdb`；文件缺失 → 全部地理核验降级跳过，不报错）。

### P2-6 验收

1. GeoLite 场景：用已知 VPN 出口 IP 访问，`asn_type=hosting` 命中；
2. 改系统时区（+8 → -5）访问 → `fpb_tz_geo_mismatch` 命中；
3. pHash/MinHash 单测全绿；假阳性灰度 7 天 < 0.5%。

---

## 6. P3 — TLS + JA4 协议指纹（1-2 天）

### P3-1 上 TLS（前置条件）

当前 `serve.go:45-46` 纯 HTTP。改造：

1. env `TLS_CERT`/`TLS_KEY` 存在 → `http.ListenAndServeTLS`；同时监听 80 端口做 301 跳转（env `REDIRECT_HTTP=1`）。
2. env `ACME_DOMAIN` 存在 → 用 `golang.org/x/crypto/acme/autocert`（缓存目录 `data/acme/`）。
3. 两者都无 → 维持纯 HTTP（本地开发模式），JA4 功能随之关闭（优雅降级）。
4. `gh_id` cookie 改 `Secure=true`（ipguard.go:603，条件：当前为 TLS 模式）；启用 HSTS 中间件（仅 TLS 模式）。

### P3-2 JA4 采集

- **实现路径**（保持单二进制哲学，不引入 nginx）：
  1. `go get github.com/exaring/ja4plus`（Go 原生 JA4+ 生成；license 落地核对——若为 FoxIO 系限制，则降级为下述 stdlib 方案并只称"TLS 指纹"不称 JA4）。
  2. `net.Listen` 包装器：包装 accept 到的 `net.Conn`，在 TLS 握手前读取 ClientHello 原始字节 → ja4plus 计算 JA4 字符串 → 存 per-conn。
  3. `http.Server.ConnContext` 把 JA4 塞进 `context.Context` → handler 里 `fingerprintmiddleware` 提取 → 挂到请求属性。
  4. 降级方案（若不想引依赖）：`tls.Config.GetConfigForClient(*tls.ClientHelloInfo)` 拿 `CipherSuites/SupportedCurves/SupportedPoints/SignatureSchemes/SupportedProtos` 自算 `tlsfp_v1`（归一化排序后 SHA-256 截断）。**注意按"先归一化再哈希"原则：排序 + 剔除 GREASE 值**（JA4 的核心抗对抗设计）。
- **DDL**：`ip_fingerprints` 新增 `ja4 TEXT`；`ip_profiles` 无需改。
- **数据**：内置 `internal/infrastructure/ja4db/mapping.csv`（从 FoxIO-LLC/ja4 仓库 ja4plus-mapping.csv 拷贝，注明来源与 license）；`githubhot ja4 update` 子命令更新。

### P3-3 UA ↔ TLS 交叉核验（L1×L2 交汇点）

规则（保守，防新版本浏览器假阳性）：

1. UA 声称 Chrome/Edge/Firefox/Safari，但 TLS 指纹 ∈ 已知非浏览器栈（Go stdlib / Python requests / curl / okhttp 特征）→ `fpb_ua_tls_mismatch` **+25 分**（高置信）。
2. TLS 指纹不在任何已知集（新版本）→ 仅记录 `tls_unknown`，不计分。
3. 同一 `gh_id`/同 IP 下多个"不同设备"指纹但 TLS 指纹全同 + 行为相似 → 交给 P4 图聚类。

### P3-4 验收

1. TLS 模式启动，`curl --http1.1 -A "Mozilla/5.0 (Windows NT 10.0) Chrome/126..." https://host/api/v1/hot` → `fpb_ua_tls_mismatch` 命中（curl 的 TLS 栈 ≠ Chrome）。
2. 真实 Chrome/Safari/Edge/安卓 Chrome 各访问一次 → 零误报。
3. HTTP→HTTPS 301 生效；`gh_id` 带 Secure 属性。

---

## 7. P4 — 平台证明与高级项（3-5 天）

### P4-1 Play Integrity（Android，金标准）

**重要事实约束**：本项目 APP 走自建 APK 分发（`apks/` + DownloadManager 强更），非 Play Store 安装。因此：

- Play Integrity 可用前提：设备有 GMS + 包名已在 Play Console 关联（免费创建即可，无需上架分发）。
- 自分发安装下 `appIntegrity.appRecognitionVerdict` 通常为 `UNRECOGNIZED_VERSION`（非 Play 安装）——**这不是拒绝条件**；真正校验的是：
  - `requestDetails.requestHash` == 服务端下发 nonce（防重放）；
  - `requestDetails.timestampMillis` 新鲜（< 10 min）；
  - `appIntegrity.certificateSha256Digest` ∈ 期望集（CI 已注入 APK 签名期望值，服务端 env `APP_EXPECTED_CERT_SHA256` 同值）；
  - `deviceIntegrity.deviceRecognitionVerdict` 含 `MEETS_DEVICE_INTEGRITY`（root/虚拟机不合格；`MEETS_BASIC_INTEGRITY` 降级计半分）。
- 无 GMS / Play Console 未配置 → **降级路径**：APP 上报 `PackageInfo.signingInfo` 的签名证书 SHA-256 + `ThreatDetect.kt` 检测结果（root/emulator/hook，现在结果根本不上报）→ 服务端比对签名。此为银标准（可被 hook 伪造，但成本高于现状）。

**服务端改造**：

1. 新端点：
   - `GET /api/v1/app/attest/challenge` → `{nonce, expires_at}`（nonce = 32B 随机 hex；HMAC 存内存 map，TTL 10min，绑当前 gh_id）。
   - `POST /api/v1/app/attest/verify` → body `{token}` 或降级 `{cert_sha256, threat: {...}}` → 返回 `{level: "play_integrity"|"signature_fallback", verdict: {...}}`；结果 JSON 存 `ip_fingerprints.attestation` 新列。
2. Google 验证调用：`POST https://playintegrity.googleapis.com/v1/{package}:decodeIntegrityToken`，鉴权用 GCP service account（env `PLAY_INTEGRITY_SA_JSON` 指向 JSON 文件路径，scope `https://www.googleapis.com/auth/playintegrity`）。走现有 safehttp 出站（SSRF 防护）。
3. env：`PLAY_INTEGRITY_PACKAGE`、`PLAY_INTEGRITY_SA_JSON`、`APP_EXPECTED_CERT_SHA256`、`ATTEST_REQUIRED=0|1`（1 = attestation 失效的设备进入只读降级模式）。

**Android 改造**（独立仓库，同步发版）：

1. `build.gradle` 加 `com.google.android.play:integrity`（版本以官方文档为准）。
2. 新建 `AttestationClient.kt`：challenge → IntegrityManager 请求（requestHash=nonce）→ token POST verify；失败/无 GMS → 降级路径（签名 + ThreatDetect 上报）。
3. `AppIntegrity.kt` 的客户端自检保留（防御纵深），结果一并随降级路径上报。

**测试**：Google 响应解析器用 mock JSON 单测（构造 PLAY_RECOGNIZED / UNRECOGNIZED_VERSION / requestHash 不符三用例）；真机验证为手动验收项。

### P4-2 WebAuthn 通行密钥（管理端）

- 库：`go get github.com/go-webauthn/webauthn`（FIDO2 认证一致）。
- env：`WEBAUTHN_ENABLED=1`、`WEBAUTHN_RP_ID`（=域名）、`WEBAUTHN_ORIGIN`（=完整 origin）。
- 端点：`POST /admin/passkey/begin-register|finish-register`（需已登录）、`POST /admin/passkey/begin-login|finish-login`（免密登录入口）。
- 存储：新表 `admin_credentials (id, credential_id BLOB UNIQUE, public_key BLOB, attestation_type, sign_count, transports, created_at)`。
- 登录策略：passkey 成功 → 复用现有 `gh_admin_session` 会话（auth.go:227）并绑定 IP（同 P0-2）；Argon2id 密码保留为后备（env `WEBAUTHN_ONLY=1` 可关密码）。
- 前端：`web/src/admin/` 加 passkey 管理页（注册/查看/删除）；登录页加"通行密钥"按钮，`navigator.credentials.get()` 流程，不支持的浏览器隐藏按钮。
- **验收**：Chrome/安卓 Chrome 各注册一把 passkey，登录成功；错设备断言失败。

### P4-3 ALTCHA PoW（请求成本）

- **优先自研**（约 60 行，避免外部依赖）：
  1. `GET /api/v1/altcha/challenge` → `{algorithm:"SHA-256", challenge:"base64(maxAge:difficulty:salt:hmac)", maxage, difficulty}`；hmac = HMAC-SHA256(salt+maxAge, ALTCHA_SECRET)。
  2. 客户端（前端 JS，`web/src/lib/altcha.js`）：解 base64 得 payload，暴力 nonce 使 `SHA-256(challenge+nonce)` 二进制前导零 ≥ difficulty，提交 `(challenge, nonce, signature=HMAC(fp+gh_id))`。
  3. 服务端 `POST /api/v1/altcha/verify`：验 HMAC、重算 PoW、查 maxAge；signature 绑 fp 防跨指纹重放。
- **挂载点**：先挂 `/api/v1/fp/report`（高频上报口，最划算）；difficulty env `ALTCHA_DIFFICULTY` 默认 12（约毫秒级），可按一层限流压力自适应（命中率 > 阈值时 +2）。
- APP 端同步实现（Kotlin 同算法）。
- **验收**：不开 PoW 时行为不变；开启后无 PoW 的裸请求 401+计分；带 PoW 请求延迟增加 < 50ms。

### P4-4 图聚类（马甲合并，先连通分量版）

- **节点**：`ip_fingerprints.fp`。**边**（任一成立，30 天窗口）：
  1. 共享 IP（≥2 个相同 IP）；
  2. pHash 汉明距离 ≤ 10 或 MinHash Jaccard > 0.8；
  3. `clock_skew_ppm` 差 < 5 且 behavior 余弦相似 > 0.9；
  4. TLS 指纹相同 + UA 声称互异（P3 数据）。
- **算法**：每日 cron 跑一次 BFS 连通分量（数据量 < 1e5 无需 Louvain）；簇大小 ≥ 3 → 标记 review，簇内任一成员被 ban → 全簇连坐系数 ×1.5（复用 BannedAmong 通道 :620）。
- **DDL**：`ip_fingerprints.cluster_id INTEGER`；新表 `fp_clusters (id INTEGER PRIMARY KEY, member_fps TEXT/*JSON数组*/, size INTEGER, first_seen TEXT, reason TEXT)`。
- **管理端**：IP 防护面板加"集群视图"（Tab 列表 + 成员下钻）。

### P4-5 行为生物特征采集（前端埋点，规则版先行）

- `web/src/lib/behavior.js`：
  - 鼠标：mousemove 采样（节流 16ms），滑窗 500 事件；特征 = 速度均值/方差、曲率均值、**jerk（加加速度）方差**、方向变化率。
  - 击键：keydown/keyup 时间戳，滑窗 100 键；特征 = dwell（按下时长）均值/方差、flight（键间）均值/方差。
  - 上报节流：每 5 分钟或页面卸载时 POST（并入 fp/report 的 `behavior` 字段，结构化数组）。
- **服务端规则版**（不引 ML）：
  - 速度方差 == 0（完全匀速）或曲率 == 0（纯直线）→ `behavior_machine` +15；
  - dwell 方差 == 0（机械击键）→ 同上；
  - 跨指纹 behavior 余弦相似 > 0.9 → 图聚类边（P4-4）。
- 存储：`ip_fingerprints.behavior JSON`。
- **隐私声明**：About 页与 `/llms.txt` 注明采集范围（交互时序统计，不含内容）。

### P4-6 时钟偏移（物理层信号）

- 前端：会话存活期间每 60s 采样 `Date.now() - performance.now()`，滑窗 20 点最小二乘回归斜率 → `clock_skew_ppm = slope × 1e6`；随 fp/report 上报。
- 服务端：存 `ip_fingerprints.clock_skew_ppm`；同物理机多马甲 → 斜率高度一致 → 图聚类边（P4-4 规则 3）。
- 注意：浏览器节流（后台标签）会造成假点 → 只取 `document.visibilityState==='visible'` 时的样本。

### P4-7 验收

1. 同一物理机 Chrome + Chrome 隐身窗口 + patchright 三身份访问 → 图聚类合并为 1 簇；
2. Play Integrity mock 单测全绿；真机（有 GMS）verify 成功返回 verdict JSON；
3. passkey 全流程可登录；ALTCHA 开关行为符合预期。

---

## 8. P5 — 行为机器学习分类（3-4 天开发 + 2-4 周数据积累期）

> 前置：P4-5 行为采集已上线并积累数据；P5-1 标注管道不依赖行为数据，可在 P4 期间并行开发。
> 核心纪律：**所有 ML 分数一律先 shadow（复用 `FP_SCORE_SHADOW`），达到指标门槛且灰度 7 天假阳性 < 0.5% 后才允许计分**。规则版（P4-5 服务端规则）保留为兜底，ML 是叠加不是替换。

### P5-1 标注体系与数据管道（一切模型的前提）

- **DDL**：`fp_labels` 表见 §10.1 汇总（fp + label + source + confidence）。
- **弱标签**（source=rule，每日 cron 生成刷新）：曾入三层封禁记录 → `bot`(0.7)；P1 灰度期命中 ≥2 项 `fpb_*`/`botd_*` → `bot`(0.6)；近 30 天零 flag 且有行为数据 → `human`(0.8)；其余 `uncertain`。
- **金标签**：管理端指纹下钻页加"标注"操作（`POST /api/v1/admin/fp/label`，body `{fp, label, notes}`），source=admin、confidence=1.0，覆盖弱标签。标注操作同时给 P6 GNN 提供训练标签。
- **导出**：`githubhot ml export --out data/ml/behavior.jsonl`，每行 `{fp, features:{...}, label, weight=confidence}`。特征清单（全部统计量，不含原始事件）：behavior 各特征（dwell/flight 均值方差、速度均值方差、曲率、jerk 方差、方向变化率、事件量）、`entropy_bits`、`stability`、`clock_skew_ppm`、灰度 flags 命中计数、会话时长、近 30 天 IP 数。
- `data/ml/`、`data/models/`、`scripts/ml/.venv` 加入 `.gitignore`。

### P5-2 Isolation Forest 无监督异常分（不需要标签，最早可上线）

- 自研纯 Go（iTree 递归随机分割，~200 行，零依赖；或用 e-XpertSolutions/go-iforest，41★ 纯 Go——二选一，倾向自研以免外部依赖）。
- 输入：P5-1 特征清单，z-score 标准化（scaler 参数随模型 JSON 存储）。
- 调度：每日 cron（04:00，独立于 HOT_CRON）对近 7 天活跃指纹训练+打分 → 写 `ip_fingerprints.anomaly_score`。
- 阈值：99.5 分位仅记录；99.9 分位入管理端 review 队列。**永远 shadow 不直接计分**，主要作为 P5-3/P5-4 监督模型的输入特征。

### P5-3 监督模型 v1：逻辑回归（Go 原生前向）

- **训练**：`scripts/ml/train_behavior.py`。Python 环境建在项目内：`scripts/ml/` 下 `python -m venv .venv`，`requirements.txt` 锁版本（scikit-learn/numpy/pandas）。**按时间切分**：前 80% 训练 / 后 20% 验证（防时间泄漏），输出 AUC 与混淆矩阵。
- **模型文件**：`data/models/behavior_lr_v1.json`：`{version, feature_names[], mu[], sigma[], weights[], bias, metrics:{auc,fpr}, trained_at, train_size, active:true}`。
- **Go 推理**：`internal/domain/fpmath/lr.go`——标准化 + 点积 + sigmoid，~30 行零依赖；serve 内 goroutine 监听 `ML_MODEL_DIR` 目录 mtime 热加载。
- **输出**：`behavior_ml_score`（0-1）入 flags，shadow 纪律同 §0.7。**出 shadow 门槛：验证集 AUC ≥ 0.85 且 FPR ≤ 0.5%**。
- **训练触发**：`githubhot ml export` → `scripts/ml/run_train.ps1`（封装 venv 激活 + 训练 + 导出），每周一次（服务器 cron / Windows 计划任务，不进 Go 进程）。

### P5-4 监督模型 v2：GBDT 树模型（条件触发升级）

- **触发条件**：LR 验证 AUC < 0.85，或标注样本 > 5 万。
- **训练**：LightGBM（追加进 requirements）；**导出**：`scripts/ml/export_lgbm_json.py` 把 `booster.dump_model()` 转纯 JSON 树 `{trees:[{feature,threshold,left,right,leaf_value}...]}`。
- **Go 推理**：`internal/domain/fpmath/treemodel.go`——JSON 树遍历求和，~120 行零依赖；与 LR 共存时按模型文件 `active` 标记取生效版本。
- **红线**：禁止引入 onnxruntime（CGO，破坏纯 Go 单二进制）；禁止 Python 进入运行时。

### P5-5 持续认证（可选，P5-3 验收后评估）

- 每指纹维护行为模板（历史特征均值 μ、对角协方差 Σ）；新会话算马氏距离 `d²=(x-μ)ᵀΣ⁻¹(x-μ)`，超 99 分位 → 触发 ALTCHA 难度 +4 的加强挑战而非直接拒绝。

### P5-6 漂移监控与评估闭环

- **PSI**：每特征按周算 `Σ(Aᵢ-Eᵢ)·ln(Aᵢ/Eᵢ)`（本周分布 vs 训练分布），任一特征 PSI > 0.2 → 管理端诊断页告警并触发再训练。
- 管理端"ML 诊断"页：当前模型版本与指标、混淆矩阵、PSI 表、shadow 命中分布直方图。

### P5-7 验收

1. `ml export` 输出可被 train_behavior.py 直接消费（字段对齐单测）；
2. LR 前向 Go 实现与 Python `predict_proba` 对拍，固定权重下误差 < 1e-9；
3. 注入 patchright 流量一周，`anomaly_score` 分布显著右移（均值差 > 2σ）；
4. 出 shadow 前：验证集指标达标 + 灰度 7 天假阳性 < 0.5%。

---

## 9. P6 — 图算法升级：Louvain → GNN → MIDAS（5-6 天）

> 前置：P4-4 聚类边数据积累 ≥ 2 周。本阶段把连通分量手工启发式升级为三件武器：**Louvain**（结构分层）、**GNN**（半监督节点风险）、**MIDAS**（实时涌现检测），三者与 P5 行为分正交互补。

### P6-1 图快照管道

- `githubhot ml export-graph --out data/ml/graph.jsonl`：节点 = fp / ip / ua 三类（带类型与节点特征：fp 节点挂 P5-1 特征向量，ip 节点挂 ASN 类型）；边 = P4-4 四类边 + 新增"时间共现边"（同 5 分钟窗口同 IP 出现）；边属性 `{type, weight, first_seen, last_seen}`；30 天窗口。

### P6-2 Louvain 社区发现（gonum，纯 Go）

- `go get gonum.org/v1/gonum/graph/community`（Go 科学计算标准库，纯 Go，CGO=0；落地核对该包 Louvain API 细节）。
- 每日 cron：内存构图 → Louvain → 社区划分覆盖 `cluster_id`，记录模块度。
- **社区风险分**：`risk = ban成员占比 × min(1, size/10)` → `fp_clusters.risk_score`；risk > 0.5 或模块度 > 0.3 的社区入 review 队列。
- **相对连通分量的增益**：连通分量被桥接节点并成巨团；Louvain 按模块度分层，能区分"核心团伙"与"边缘关联"。
- **验收**：单测构造"双团伙 + 单桥接节点"图，Louvain 正确切分而连通分量并为一团。

### P6-3 GNN 离线训练（PyTorch Geometric，Go 零推理）

- **环境**：`scripts/ml/requirements-graph.txt`（torch CPU 版 + torch_geometric），复用项目内 venv。
- **模型优先级**：GraphSAGE（归纳式，新节点无需重训——首选）→ CARE-GNN（参考 DGFraud 762★ 实现，过滤低相似邻居、抗马甲混入）→ GCN 基线。
- **半监督设定**：`fp_labels` 中 admin/rule 标签节点作训练标签，无标签节点参与消息传递；任务 = fp 节点二分类（bot 概率）。
- **输出**：`githubhot ml import-gnn data/models/graph_gnn_v1.json`——每 fp 写回 `gnn_score`（bot 概率）与 64 维 `gnn_embedding`，同时写模型卡（指标/训练集规模/日期）。
- **冷启动**：新 fp 未被上次训练覆盖 → 取其 Louvain 社区内已评估节点 gnn_score 均值兜底，下次训练覆盖。
- **频率**：每周随 run_train.ps1 跑；图 < 1e5 节点，CPU 训练分钟级。
- **红线**：GNN 只离线训练导出分数，Go 进程零推理、零 Python 依赖。

### P6-4 MIDAS 流式边异常（实时层，抓正在发生的协同攻击）

- 算法：MIDAS（WSDM 2020）——维护两张 count-min sketch（当前时间片计数 / 历史累计计数），对边 (ip→fp) 算卡方型统计量 `Σ(aᵢ-sᵢ)²/(2·sᵢ)`，σ 超限 = "突然涌现的稠密子图"。参考 Stream-AD/MIDAS（777★，C++），Go 自实现核心 ~300 行。
- **挂载**：一层防护旁（`ipguard.go:449` 附近），每请求更新边计数，时间片 60s；`midas_score` 存内存（随一层重启丢失，可接受）。
- **动作**：>3σ → 实时积分 +15（先 shadow，仍受 `FP_SCORE_SHADOW` 控制）；>5σ → 直接进 review 队列。
- **验收**：模拟 20 IP × 20 fp 两个时间片内互联 → 告警命中；平稳流量一周零误报。

### P6-5 管理端图可视化

- `web/` 加 `d3-force`（npm 装项目内）；`GET /api/v1/admin/ipguard/graph` 返回当前图（节点/边/分数，按风险截断上限 500 节点）。
- 集群视图升级为力导向图：节点色 = gnn_score（绿→红）、节点大小 = 度、点击下钻指纹详情、就地联动 P5-1 标注按钮。

### P6-6 融合规则（正交性检查 + 接入三层）

- 上线时验证 `behavior_ml_score` 与 `gnn_score` 皮尔逊相关 < 0.6（≥ 0.6 说明特征泄漏，回炉重做）。
- 融合：两者均 > 0.8 → severe 违规（走三层积分 :349 通道）；单项超 → review 队列 + 低权计分。

---

## 10. 数据模型与 API 变更汇总

### 10.1 DDL 汇总（迁移脚本一次执行，幂等写法参照现有迁移）

```sql
ALTER TABLE ip_fingerprints ADD COLUMN ja4 TEXT;
ALTER TABLE ip_fingerprints ADD COLUMN canvas_phash TEXT;
ALTER TABLE ip_fingerprints ADD COLUMN minhash_sig TEXT;
ALTER TABLE ip_fingerprints ADD COLUMN clock_skew_ppm REAL;
ALTER TABLE ip_fingerprints ADD COLUMN behavior TEXT;        -- JSON
ALTER TABLE ip_fingerprints ADD COLUMN attestation TEXT;     -- JSON
ALTER TABLE ip_fingerprints ADD COLUMN cluster_id INTEGER;
ALTER TABLE ip_fingerprints ADD COLUMN entropy_bits REAL;
ALTER TABLE ip_fingerprints ADD COLUMN stability REAL;

CREATE TABLE IF NOT EXISTS fp_lsh_buckets (
  band INTEGER, bucket_hash TEXT, fp TEXT, created_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_lsh_bucket ON fp_lsh_buckets(band, bucket_hash);

CREATE TABLE IF NOT EXISTS fp_clusters (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  member_fps TEXT NOT NULL, size INTEGER NOT NULL,
  first_seen TEXT NOT NULL, reason TEXT
);

CREATE TABLE IF NOT EXISTS admin_credentials (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  credential_id BLOB NOT NULL UNIQUE,
  public_key BLOB NOT NULL, attestation_type TEXT,
  sign_count INTEGER DEFAULT 0, transports TEXT, created_at TEXT NOT NULL
);

ALTER TABLE ip_profiles ADD COLUMN asn INTEGER;
ALTER TABLE ip_profiles ADD COLUMN asn_type TEXT;
ALTER TABLE ip_profiles ADD COLUMN geo_country TEXT;
ALTER TABLE ip_profiles ADD COLUMN geo_tz TEXT;
```

P5/P6 增量迁移（在上述迁移之后执行）：

```sql
ALTER TABLE ip_fingerprints ADD COLUMN anomaly_score REAL;
ALTER TABLE ip_fingerprints ADD COLUMN behavior_ml_score REAL;
ALTER TABLE ip_fingerprints ADD COLUMN gnn_score REAL;
ALTER TABLE ip_fingerprints ADD COLUMN gnn_embedding TEXT;   -- 64 维，JSON 数组
ALTER TABLE fp_clusters ADD COLUMN risk_score REAL;

CREATE TABLE IF NOT EXISTS fp_labels (
  fp TEXT NOT NULL,
  label TEXT NOT NULL CHECK(label IN ('human','bot','uncertain')),
  source TEXT NOT NULL CHECK(source IN ('admin','rule','model')),
  confidence REAL NOT NULL,
  labeled_at TEXT NOT NULL,
  notes TEXT,
  PRIMARY KEY (fp, source)
);
```

### 10.2 端点汇总

| 端点 | 方法 | 阶段 | 鉴权 |
|---|---|---|---|
| `/api/v1/app/attest/challenge` | GET | P4 | gh_id |
| `/api/v1/app/attest/verify` | POST | P4 | gh_id |
| `/api/v1/altcha/challenge` / `verify` | GET/POST | P4 | 无（PoW 即凭证） |
| `/admin/passkey/*` | POST | P4 | 会话（注册）/无（登录） |
| `/api/v1/fp/report` | POST | P1-P4 | 现有，字段向后兼容扩展 |
| `githubhot admin seed` / `geo download` / `ja4 update` | CLI | P0/P2/P3 | — |
| `POST /api/v1/admin/fp/label` | POST | P5 | 管理会话（人工标注 human/bot/uncertain，金标签） |
| `GET /api/v1/admin/ipguard/graph` | GET | P6 | 管理会话（集群图数据：节点/边/分数，力导向可视化） |
| `githubhot ml export` / `ml export-graph` / `ml import-gnn` / `ml load-model` | CLI | P5/P6 | 训练数据导出、GNN 分数回写、模型热加载 |

### 10.3 env 汇总（同步更新 .env.example 与 README 配置表）

```
APP_SIGN_SEED=            # P0 必填，32B base64；等于默认值时 serve 拒启
APP_SIGN_SEED_GRACE=      # P0 过渡期旧 seed 列表
ADMIN_SESSION_IP_STRICT=  # P0 默认 0（/24 放宽）
HANDSHAKE_RATE_PER_MIN=   # P0 默认 30
FP_SCORE_SHADOW=          # P1 默认 1（灰度只记录）
GEOIP_DB_PATH=            # P2 默认 data/geo.mmdb
TLS_CERT= / TLS_KEY= / ACME_DOMAIN= / REDIRECT_HTTP=   # P3
WEBAUTHN_ENABLED= / WEBAUTHN_RP_ID= / WEBAUTHN_ORIGIN= / WEBAUTHN_ONLY=   # P4
PLAY_INTEGRITY_PACKAGE= / PLAY_INTEGRITY_SA_JSON= / APP_EXPECTED_CERT_SHA256= / ATTEST_REQUIRED=   # P4
ALTCHA_SECRET= / ALTCHA_DIFFICULTY=   # P4
ML_MODEL_DIR=            # P5 默认 data/models，模型 JSON 热加载目录
ML_TRAIN_VENV=           # P5 默认 scripts/ml/.venv，Python 训练环境（项目内）
```

---

## 11. 依赖清单（落地时逐项核对 LICENSE 并回填本表）

| 依赖 | 用途 | 引入阶段 | 备注 |
|---|---|---|---|
| `@fingerprintjs/botd` | 前端自动化检测 | P1 | MIT，1478★ |
| `oschwald/geoip2-golang` | mmdb 读取 | P2 | 数据用 ip-location-db（CC BY 4.0，须致谢） |
| `exaring/ja4plus` | Go JA4 生成 | P3 | license 待核；受限则用 stdlib 降级方案 |
| `go-webauthn/webauthn` | 通行密钥 | P4 | 1343★，FIDO2 认证 |
| `golang.org/x/crypto/acme/autocert` | 证书自动签发 | P3 | — |
| 自研 | pHash/MinHash/LSH/熵/ALTCHA | P2/P4 | 优先自研，算法见 §5 与 P4-3 |
| `gonum.org/v1/gonum` | Louvain 社区发现 | P6 | 纯 Go 科学计算标准库；graph/community 的 Louvain API 落地核对 |
| `d3-force`（web/） | 集群力导向可视化 | P6 | 前端，npm 装项目内 |
| Python：scikit-learn / lightgbm / torch+PyG | 离线训练（scripts/ml/.venv，项目内 venv，不入运行时） | P5/P6 | CPU 版即可；requirements 锁版本 |
| 自研（ML 推理） | LR 前向 / GBDT 树遍历 / iForest / MIDAS sketch | P5/P6 | 全部纯 Go，保持 CGO=0 单二进制 |

情报参考（**不引入**）：camoufox（12.3k★，内核级反检测）、patchright（4.8k★）、fingerprint-suite（2.6k★）、untidetect-tools（2k★，敌方装备目录）、CreepJS（2.5k★，思路参考禁抄码）、curl_cffi（6.7k★，攻击侧 TLS 模拟）。

---

## 12. 测试计划与总验收

### 12.1 单测（go test ./... 必须全绿）

- `internal/domain/fpmath`：pHash 汉明距离、MinHash 签名与 Jaccard 估计、LSH 分桶、熵计算、EWMA、线性回归斜率；P5 起新增——LR 前向（与 Python `predict_proba` 固定权重对拍，误差 < 1e-9）、GBDT JSON 树遍历（与 LightGBM predict 对拍）、MIDAS sketch 计数与告警阈值；P6 起新增——Louvain 桥接团伙切分用例（gonum）。
- `internal/interfaces/httpapi`：UA↔TLS 矛盾规则、flags 计分映射、altcha challenge/verify 往返、attestation mock 三态解析、管理会话 IP 校验、标注端点权限。
- 前端：`web/src/lib/phash.js`、`altcha.js` 用 `node --test` 跑纯函数用例。
- Python（不进 CI 门槛，训练前自检）：`scripts/ml/selftest.py`——数据切分无泄漏、特征清单与 Go 侧对齐。

### 12.2 对抗性手动验收（每阶段末执行，结果记入交付说明）

| 对手 | 工具 | 期望 |
|---|---|---|
| 裸脚本 | curl + Chrome UA | P3：ua_tls_mismatch 命中 |
| JS 层 stealth | `npx patchright` 访问 | P1：≥2 项 fpb_*/botd_* 命中；P5 后：anomaly_score 右移 > 2σ |
| 内核级反检测 | camoufox（可装则测） | ①②允许漏，⑤⑥至少一项命中 |
| 换指纹轮换 | 手动改 canvas 毒化参数 | P2：pHash/MinHash 关联 + rotation_detected |
| 多马甲 | 同机多浏览器身份 | P4：图聚类合并同簇；P6：Louvain 切分核心团伙，GNN 分数高 |
| 协同攻击模拟 | 20 IP × 20 fp 短时互联 | P6：MIDAS 1-2 个时间片内告警 |
| 正常用户 | Chrome/Edge/Safari/安卓 Chrome | 全程零命中（假阳性红线 0.5%） |
| ML 模型质量 | 验证集 + shadow 期 | AUC ≥ 0.85、FPR ≤ 0.5% 才出 shadow；行为分与 GNN 分相关性 < 0.6 |

### 12.3 交付物清单

1. 代码 + 全部测试绿；
2. `.env.example`、README 配置表、`docs/architecture.md` 增补"指纹与防护"章节；
3. 每阶段一条 commit（中文 scope 前缀）；
4. 交付说明文档：各对抗验收的实际结果、灰度面板截图路径、已知假阳性清单；
5. P5/P6 追加：`scripts/ml/` 训练管道（requirements 锁版本 + run_train.ps1）与模型卡（每个模型 JSON 附特征清单、指标、训练集规模、训练日期）。

---

## 13. 明确不做（边界）

1. 不做 mTLS 客户端证书（WebAuthn 已覆盖管理端场景，APP 场景由 Play Integrity 覆盖）；
2. 不做 VDF（ALTCHA PoW 对当前威胁面足够，VDF 留待 PoW 被绕过再评估）；
3. 不做在线/实时 GNN 推理，也禁止 Python 与 CGO 依赖进入运行时（GNN 仅离线周训练导出分数，Go 侧只读——保持纯 Go 单二进制）；
4. 不做深度序列模型（LSTM/Transformer 级行为建模，当前数据量不支持；监督侧止步 GBDT，序列特征以滑动窗口统计量近似）；
5. 不动业务层（榜单/日报/信源/LLM 流水线）；
6. 不重构现有三层 IP 防护与积分通道（新检测全部挂现有通道）；
7. Android 侧改动在独立仓库 GithubHot-App 执行，本文档只约束其行为契约（attest 端点、ALTCHA 算法、seed 构建期注入）。

---

## 附录 A：关键算法速查

- **pHash**：32×32 灰度 → DCT-II → 8×8 低频（跳过 DC）→ 中位数阈值 → 64bit。
- **MinHash**：`h_i(x)=(a_i·x+b_i) mod p`，p=2^61−1，k=128；LSH：16 带 × 8 行。
- **熵权**：`w=-log2(c(v)/N)`；指纹置信 `bits=Σw`；封禁系数 `min(1, bits/40)`。
- **EWMA**：`S_t=α·X_t+(1-α)·S_{t-1}`，α=0.3。
- **时钟偏移**：`offset(t)=Date.now()-performance.now()`，20 点最小二乘斜率 ×1e6 = ppm。
- **getter 时序**：N=1e5 循环摊薄；判定 `p50_target > max(1µs, 20×p50_base)`；3 轮取中位数；只记录不计分（灰度）。
- **马氏距离**（持续认证）：`d²=(x-μ)ᵀΣ⁻¹(x-μ)`，模板 = 历史特征均值，协方差取对角近似。
- **PSI**（漂移监控）：`Σ(Aᵢ-Eᵢ)·ln(Aᵢ/Eᵢ)`，> 0.2 触发再训练。
- **MIDAS**（流式边异常）：双 count-min sketch（当前时间片 / 历史累计），卡方统计量 `Σ(aᵢ-sᵢ)²/(2·sᵢ)`，σ 超限告警（WSDM 2020 论文，参考 Stream-AD/MIDAS 777★）。
- **GNN 部署形态**：训练（PyG GraphSAGE 优先，周级）产出 `gnn_score` + 64 维嵌入写库；Go 零推理；冷启动用 Louvain 社区均值。
- **社区风险分**：`risk = ban成员占比 × min(1, size/10)`，> 0.5 入 review。
- **归一化再哈希原则**（贯穿所有自研指纹）：凡对攻击者可控输入做指纹，先排序归一化、剔除 GREASE/噪音值，再哈希——JA4 对 JA3 的核心改进，同样适用于 fingerprint.js:188 的分量融合顺序。

