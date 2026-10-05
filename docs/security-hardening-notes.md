# 反伪造防护体系 — 交付说明与验收记录

> 对应需求：`docs/security-fingerprint-spec.md`（唯一需求来源）。
> 本文档记录**每个阶段的实现范围、验证证据、部署注意事项**，随阶段推进追加。
> 提交纪律：每阶段一条中文 commit（scope 前缀）；主仓库**只本地提交、不 push**（push 会触发部署流水线）。

## 总览

| 阶段 | 范围 | 状态 | 提交 |
|---|---|---|---|
| P0-1 | APP 签名种子停止下发 + serve 强制非默认 | ✅ 完成 | `a1461bd` |
| P0-2 | 管理会话绑定来源网段 | ✅ 完成 | `7fc06da` |
| P0-3 | 握手通道独立滑动窗口限流 | ✅ 完成 | 见 git log `P0-3` |
| P0-4 | 检测 flag 命中统计（后端聚合 + 面板区块） | ✅ 完成 | 见 git log `P0-4` |
| P1 | 指纹浏览器识别（六路采集 + BotD + 计分接入） | ✅ 完成（2 处按实测调整，见下） | 见 git log `P1` |
| P2 | 算法升级（pHash / MinHash+LSH / 熵权 / 稳定性 / GeoIP） | 🔶 进行中：P2-1 完成、P2-2 域包完成待接线，P2-3/P2-4/P2-5 未开工 | 见 git log `P2-1` |
| P3 | TLS + JA4 协议指纹 | ⏳ 待办 | — |
| P4 | 平台证明与高级项（Play Integrity / passkey / ALTCHA / 图聚类 / 行为采集 / 时钟偏移） | ⏳ 待办 | — |
| P5 | 行为机器学习（标注 / iForest / LR / GBDT / PSI） | ⏳ 待办 | — |
| P6 | 图算法升级（快照 / Louvain / GNN / MIDAS / 融合） | ⏳ 待办 | — |
| §10 | 管理端可视化整合（review 队列 / 集群图 / ML 诊断） | ⏳ 待办 | — |

---

## P0 — 现有硬伤修复

### P0-1 APP 签名种子停止下发 + serve 强制非默认

**问题**（硬伤 B）：`/api/v1/site/config` 公开返回 `session_seed`，历史默认值 `gh-dev-seed-v1`；
客户端用 `sessionKey = HMAC(seed+":"+fp, "gh-session-v1")` 派生签名密钥 → 种子公开等于密钥公开，
签名机制对任何人可复现。

**实现**

- `spa.go`：`/api/v1/site/config` 删除 `session_seed` 字段（保留 `banned` / `force_upgrade_url`）。
- 种子只从环境变量读取：`APP_SIGN_SEED`（兼容旧名 `APP_SESSION_SEED`）；`AppGuard` 支持多种子验签
  （主种子 + `APP_SIGN_SEED_GRACE` 过渡期旧种子），签名与会话令牌都按种子逐个比对。
- `config.CheckAppSignSeed()`：serve 模式种子为空或等于 `gh-dev-seed-v1` → 拒绝启动（run/mcp 降级为警告）。
- 过渡期语义：`APP_SIGN_SEED_GRACE` 非空时，验签无法通过只记 `app-sign-unverified`（0 分观察），
  否则记 `app-sign-invalid`（60 分强证据）。原因：存量 APP 仍用握手期缓存种子，新购机客户端会本地
  随机生成种子，按 60 分计会让正常用户被连续 401 攒到封禁。
- 新增 CLI `githubhot admin seed`（32 字节 base64url 随机值）。

**部署注意（重要）**：生产 `.env` 目前**没有** `APP_SIGN_SEED`（此前一直用出厂默认值），
因此升级后 serve 会拒绝启动。迁移步骤：

1. `githubhot admin seed` 生成种子 → 写入 `.env` 的 `APP_SIGN_SEED`；
2. 过渡期同时配 `APP_SIGN_SEED_GRACE=gh-dev-seed-v1`（存量 APP 的缓存种子仍是它）；
3. APP 端改为**构建期注入**同名种子（Gradle `buildConfigField` / CI Secret）后发版；
4. 发版完成、观察期（建议 ≤14 天）结束后清空 `APP_SIGN_SEED_GRACE`（之后旧种子验签失败将按 60 分计）。

**验证**

- 单测：`internal/config`（种子来源/优先级/grace 解析/启动校验 4 例）、
  `httpapi`（多种子验签命中下标、grace 开关、grace 下不计分、常态计 60 分、旧种子放行、
  握手接口响应不含种子字面量 6 例）。
- 活体：无种子 / 默认种子启动 → 退出码 1 并打印原因；配置真实种子后
  `GET /api/v1/site/config` 返回 200 且响应体无种子字段。

### P0-2 管理会话绑定来源网段

**问题**：`admin_sessions` 表存了会话签发 IP，但 `checkAdminSession` 丢弃不校验 → Cookie 被拷走后
可在任意 IP 使用，直到过期。

**实现**

- 新域包 `internal/domain/netident`（纯算法）：`SameScope(a, b, strict)` — IPv4 比 `/24`、
  IPv6 比 `/64`，地址族不同不放宽，IPv4-mapped IPv6 先归一。
- `checkAdminSession`：签发 IP 与当前请求 IP 不在同一作用域 → **立即注销会话** +
  记 `admin-session-ip-mismatch`（I 强证据类）80 分 + 401。
- 分值刻意取 80（与 `device-mismatch` 同档）而非 100：单次不符不触达强证据单类封禁线，
  否则管理员换宽带 / 手机切基站会被自己封在登录页外（被封 IP 连 `/admin/login` 都是 404 → 自锁）。
  会话已即时注销，安全效果已达成；重复跨网段（去重窗口外的同类事件）才升级封禁。
- 登录侧口径对齐：`adminLogin` 改用 `clientIPFromRequest`（受信代理下取真实客户端 IP），
  否则会话记录的是代理 IP、校验用的是真实 IP，正常管理员会被自己踢下线。
- `ADMIN_SESSION_IP_STRICT=1` 收紧为完全一致；日志只打印会话 ID 前 8 位（凭证不落日志）。

**验证**

- 单测：`netident` 3 例（v4/v6/混合族）；`httpapi` 6 例（同 /24 放行、跨 /24 注销+计分、
  严格模式、空 IP 不校验、5 分钟去重不放大、去重窗外持续使用升级封禁、登录口径一致）。
- 活体（8792）：`203.0.113.7` 登录 → 同 /24（`.99`）访问 200 → 跨 /24（`198.51.100.7`）访问
  401「会话与访问来源不符，已注销」→ 原 IP 复用同一 Cookie 401；`ipguard/summary` 事件表出现
  该条 80 分事件（Detail 含签发/当前 IP）。

### P0-3 握手通道纳入限流

**问题**：`/api/v1/site/config` 免签（APP 冷启动必需）且会触发指纹归档写库，此前只受全站阈值
（150/min 记分、600/min 即时封）约束 → 100/min 级重放可持续刷写库。

**实现**

- `IPGuard.handshakeAllow`：60s 滑动窗口、按 IP 独立计数（`ipWindow.hsTimes`），
  与全站速率窗口互不干扰；超限返回 429 并记 `handshake-rate`（P 弱证据）30 分。
- 阈值 env `HANDSHAKE_RATE_PER_MIN`（默认 30）；回环/内网白名单不设限（本地开发防自锁）。
- 挂在 IPGuard 中间件内、**先于** AppGuard 生效：APP 握手请求的指纹归档写库也在限流之后。

**验证**

- 单测 5 例：超限 429 + 计分（单条不封）、窗口独立性（双向）、阈值配置与非法值回落、
  本机豁免、正常客户端节奏零事件。
- 活体（8792，阈值 30）：连续 60 次请求 = **200×30 + 429×30**；事件表见
  「握手通道 31 req/min（上限 30）」；同 IP 访问 `/api/v1/hot` 仍 200；零封禁。

### P0-4 检测 flag 命中统计（为 P1/P2 灰度观察铺路）

**实现**

- 新域包 `internal/domain/fpstats`（纯算法）：`AggregateFlags(samples, since, limit)` —
  窗口过滤、同指纹内 key 去重、`hits = Σ 指纹上报次数`、`affected_fps = 涉及指纹数`，
  排序（hits↓ → fps↓ → key↑）、Top-N 截断。
- 存储端口新增 `ListFingerprintsSince(ctx, since, limit)`（sqlite：`last_seen >= ?`，RFC3339 文本比较）。
- `GET /api/v1/admin/ipguard/summary` 追加 `flag_stats: [{key, hits, affected_fps}]`（近 7 天 Top-15，
  向后兼容追加字段）。
- 前端：`web/src/admin/components/FlagStatsCard.vue`（纯 SVG 横向条形，零图表库依赖）、
  `web/src/admin/flagLabels.js`（flag 中文说明，与 IP 下钻共用同一份文案）；
  `AdminIPGuard.vue` 接入卡片 + flag 筛选联动（点击条目过滤下方指纹列表，chip 可清除）。

**验证**

- 单测：`fpstats` 5 例（计数排序、窗口过滤、去重与空 key、Visits 下限与 limit、空输入）；
  `httpapi` 4 例（聚合正确、空库空数组、Top-15 截断、防护未启用时旧契约不变）。
- 活体（8792）：POST 4 条带 flag 的指纹上报 → `flag_stats` 返回
  `headless-ua 3 次 / 3 指纹` 等；浏览器（playwright 实测）打开 `/admin/ipguard`：
  - 卡片渲染 5 行条形（中文标签 + `N 次 / M 指纹`），条形宽度按命中数归一（首行 170px）；
  - 点击「无头浏览器」→ 下方指纹表行数 4 → 3、出现 chip「🧪 无头浏览器 ×」、该行高亮；
  - 再次点击 → chip 消失、行数回到 4、高亮清除；页面零 JS 报错。

---

---

## P1 — 指纹浏览器识别

前端采集模块落在 `web/src/lib/fp/`，全部走既有 `/api/v1/fp/report` 的 flags 通道（**表结构零改动**），
探测（DOM/浏览器相关）与判定（纯函数）分离，纯函数部分用 `node --test` 覆盖（`cd web; npm test`）。

### P1-1 干净环境对照

- **Worker-Canvas**：主线程渲染像素 vs Worker 内 `OffscreenCanvas` 渲染像素；绘图指令用
  `drawInstr.toString()` 注入 Worker 源码，两侧**同一份源码**（避免指令漂移导致对照失真）。
- **iframe 参照**：运行时插入 `about:blank` iframe（独立 realm 与原型链），逐项比对
  `navigator.webdriver/plugins/mimeTypes/hardwareConcurrency/deviceMemory/languages/userAgent/platform`
  与 `screen.colorDepth`。
- **音频对照**：同一 chirp 两次独立渲染必须逐样本一致；`getChannelData` 与 `copyFromChannel`
  两条读取路径的样本和必须一致（读取层 patch 会露馅）。
- flag：`fpb_canvas_diverge` / `fpb_iframe_diverge` / `fpb_audio_diverge`（各 15 分，灰度期 0 分）；
  不可用时记 `*_check_unsupported`（不计分）。

### P1-2 能力—声明核验

- **GPU 能力集**：`web/src/lib/fp/gpu-capabilities.json` 收 6 档（NVIDIA/AMD/Apple/Intel/移动/软件），
  每档给 `MAX_TEXTURE_SIZE` 合理下限；只做**单向**判定（声明档位高于实测能力 → `fpb_gpu_claim_mismatch`），
  避免新驱动/新架构造成假阳性。软件光栅化单独记 `fpb_gpu_software`（只记录）。
- **字体实测**：见下方"规格调整 ①"。
- **核数基准**：1e7 次整数微基准耗时分档 vs `hardwareConcurrency` 声明档位，只抓极端矛盾
  （高档声明+极慢 / 低档声明+极快）→ `fpb_cores_claim_mismatch`（只记录）。

### P1-3 JS 拦截取证

- **accessor 原生性取证（核心，确定性）**：目标 accessor（`navigator.webdriver/plugins/languages/
  hardwareConcurrency/userAgent/platform/permissions`、`screen.colorDepth/width`）在原型上的 getter
  必须是原生实现（`Function.prototype.toString` 含 `[native code]`、name/length 与原生签名一致），
  否则记 `fpb_getter_patched:<属性>`。
- **native function 取证**：6 个热点函数（`toDataURL` / `getImageData` / `fillText` / `getParameter` /
  `RTCPeerConnection` / `Function.prototype.toString`）同判据 → `fpb_native_fn_tamper`。
- **错误栈版本指纹**：`Error.stack` 格式家族（V8 `at fn (...)` vs SpiderMonkey/JSC `fn@...`）与 UA
  声称内核交叉 → `fpb_stack_version_mismatch`（只记录）。
- **读取耗时侧信道**：见下方"规格调整 ②"。

### P1-4 BotD

- `web/package.json` 增 `@fingerprintjs/botd`（MIT，2.0.0，项目内安装）；动态 `import()` 加载，
  独立 chunk（12.3KB / gzip 4.1KB），不进主包解析路径。
- 结果展开为 `botd_<botKind>`（如 `botd_headless_chrome`）与 `botd_<detector>_1`（如
  `botd_web_driver_1`）两类 flag，逐项去重。

### P1-5 服务端计分接入（+ 一条既有缺陷修复）

- `detectFlagScore`：既有环境核验走原表；P1 计分项（canvas/iframe/audio 对照分歧、GPU 能力矛盾、
  原生函数篡改）各 +15，但**灰度期（`FP_SCORE_SHADOW` 默认开）一律 0 分只记录**；规格列为
  "仅记录"的项（getter 取证/时序、栈版本、核数、字体矛盾、软件光栅、`*_unsupported`、`botd_*`）
  即使关闭灰度也不计分。
- **顺带修掉一个既有点分缺陷**：旧实现对**未登记的 flag 兜底 `env-incoherent` 的 30 分**——
  客户端自报任意新键即可直接拿分。现在未知键一律 0 分只记录（有回归测试锁死）。
- getter 取证/时序 flag 按属性细分 kind（`env-flag:fpb_getter_patched:navigator.webdriver`），
  不同属性是独立证据，不被 5 分钟去重压成一条。

### 规格调整（都有实测数据支撑，非偷懒省略）

**① 字体核验：规格的 `document.fonts.check` 与 `measureText` 交叉不可用 → 改为 Canvas × DOM 双路径。**

实测（headless Chromium）：`document.fonts.check('16px "Consolas"')` 对**未安装**字体同样返回 `true`
（Chrome 的 check() 只回答"有没有待加载的 webfont"，不做字体族匹配），而 `measureText` 正确判定为未安装
→ 两者天然不一致，在无头/精简字体环境产生 **100% 假阳性**（实测干净环境命中 `fpb_font_claim_mismatch`，
conflicts=3）。改为 **Canvas measureText** 与 **DOM 布局 offsetWidth** 两条真实度量路径各自给出
"与回退字体宽度差"，差值落在 0.5–2px 模糊带的样本直接跳过（避免亚像素舍入噪声），两条路径结论相反才算
conflict，≥3 个 conflict 才置位。

实测结果：干净浏览器 `conflicts=0`（各字体两条路径差值几乎相同，如 Arial −9.7/−9.7）；
把 `measureText` patch 成"所有字体返回回退宽度"（典型字体伪装）后 `conflicts=4` 且 flag 命中。

**② getter 时序侧信道：实测在 Chrome 无分离能力 → 保留通道但改为确定性 accessor 取证。**

规格假设"被 patch 的属性比原生 getter 慢一个量级"。实测（headless Chromium，1e5 次循环 × 3 轮取中位数）：

| 目标 | 中位耗时 |
|---|---|
| `Object.getOwnPropertyDescriptor(Object.prototype,'toString')`（原规格基线） | 20 ns |
| `navigator.userAgent.length`（原生） | 166 ns |
| `screen.colorDepth`（原生） | 119 ns |
| `navigator.webdriver`（JS getter patch） | 79 ns |
| `navigator.webdriver`（JS getter + slice 工作） | 79 ns |
| `navigator.webdriver`（JS getter + 循环工作） | 78 ns |

结论：**JS getter 反而比原生跨对象 getter 更快**（原生 `navigator.*` 走跨对象/代理路径），且计时器被
Spectre 缓解粗化，1e5 次循环摊薄后所有目标仍落在 0 值域 → 原阈值（max(1µs, 20×基线)）永远无法命中，
若降到 3× 基线又会把全部 `navigator.*` 判为嫌疑（假阳性）。故：时序通道保留为"同类原生属性中的
相对离群"（>5× 同类中位且 >100ns）仅记录，**判定改用确定性的 accessor 原生性取证**（见 P1-3），
它与规格意图一致（杀 JS 层 patch）且零误报。

### 验证（真实浏览器实测，Chromium headless + Vite dev server 代理到沙箱实例 8792）

| 场景 | 结果 |
|---|---|
| 干净浏览器（无任何 patch） | `fpb_*` 零命中；canvas 主/Worker 像素哈希一致（`ccc4e21c`）、iframe 参照一致、音频确定性与读取一致、字体 conflicts=0、accessor 全原生、栈 v8/v8 |
| patch `Navigator.prototype.webdriver` | `fpb_iframe_diverge`（diff=[webdriver]）+ `fpb_getter_patched:navigator.webdriver` |
| 包装 `HTMLCanvasElement.prototype.toDataURL` | `fpb_native_fn_tamper` |
| patch `measureText`（字体伪装） | `fpb_font_claim_mismatch`（conflicts=4） |
| headless Chromium 端到端上报 | 服务端记录 `botd_headless_chrome` / `botd_detect_user_agent_1` / `botd_detect_app_version_1` + `headless-ua`，**全部 0 分**（灰度期） |
| 关掉灰度（`FP_SCORE_SHADOW=0`） | 5 个计分项各 +15；两条 15 分新检测不足以封禁（弱证据按类封顶 40） |

单测：前端 `npm test` 31 例全绿（cleanEnv 7 / claims 12 / forensics 8 + botd 5，含模糊带跳过、
未知键不计分等边界）；Go 侧 `go test ./...` 全绿（P1 计分 7 例）。

---

## P2 — 算法升级（进行中）

### P2-1 pHash 感知哈希同源关联 ✅

- 前端 `web/src/lib/phash.js`（纯函数）：32×32 灰度（双线性降采样）→ DCT-II（32 点基预计算，
  行/列可分离）→ 左上 8×8 低频块（跳过 DC）→ 中位数二值化 → 64bit（有效位 63 + 末位保留）→ hex16；
  另导出 `hammingDistance` / `similarPHash`。在既有 `canvasFp()` 上顺带算出，随 `canvas_phash` 字段上报
  （可选字段，旧服务端忽略）。
- 域包 `internal/domain/fpmath/phash.go`：`ParsePHash` / `PHashDistance` / `SimilarPHash` /
  `SimilarPHashCandidates`（纯函数，非法输入一律不关联）。
- 存储：`ip_fingerprints.canvas_phash` 列；新表 `fp_links(src,dst,kind,weight,first_seen,last_seen)`
  （pHash 同源 / 后续 MinHash 相似 / 物理特征 / 时间共现共用，P4-4 聚类与 P6 图快照的边表）；
  端口新增 `ListPHashCandidates` / `UpsertFPLink` / `ListFPLinks`。
- 引擎：`ReportFingerprint` 内 `linkByPHash` —— 30 天窗口扫候选、汉明距离 ≤ 10 视为同源、
  双向写边（weight=1−distance/64）、记 0 分观察事件 `fp-phash-link`。**只关联不计分**：
  相似本身不是违规，且同型号设备天然重合，是否"换脸轮换"留给 P2-4 与图聚类。
- 验证：`web/src/lib/phash.test.js` 6 例（确定性、1% 噪声距离 <10、不同图形 >30、亮度整体平移 <10、
  非法输入、hexToBits）；`ipguard_p2link_test.go` 6 例（同源写边/不同设备不关联/自环与非法哈希、
  30 天窗口、重复上报幂等、字段清洗）；`npm test` 37 例、`go test ./...` 全绿、`npm run build` 成功。

### P2-2 MinHash + LSH 域包 ✅（域包已就绪，尚未接线）

- `internal/domain/fpmath/minhash.go` + `minhash_test.go`（6 测试函数 / 33 子测试）：
  `NewMinHash(k=128)`、`Signature([]string) []uint64`、`JaccardEstimate`、
  `Bands(sig, 16, 8)`；`bits.Mul64/Div64` 安全取模 p=2^61−1，系数由 splitmix64 固定常量派生
  （跨进程一致、不依赖 math/rand 版本）。实测 20 组件改 1 个 → 估计 0.88 且共享带 5 个。
- 待接线：`ip_fingerprints.minhash_sig` 列与 `fp_lsh_buckets` 表已建（DDL 就绪），
  还差"上报时算签名 → 写桶 → 同带召回 → 精确 Jaccard > 0.8 关联"的引擎接线与端口方法。

### 待办

- P2-3 熵值加权（`internal/domain/fpmath/entropy.go` + 每日 cron 刷新 `entropy_bits` + 封禁系数
  `min(1, bits/40)` 接入 `iprisk`）；P2-4 时间稳定性 EWMA（`comp_stability`/`stability` 列已建，
  "稳定分量突变 + pHash/MinHash 关联旧指纹" → `fpb_rotation_detected` +25）；
  P2-5 GeoIP（ip-location-db mmdb + geoip2-golang + `githubhot geo download` + 时区/ASN 核验，
  `ip_profiles` 的 asn/geo_* 列已建）。
## 已知边界 / 后续项

- P0-4 的指纹列表仍受 `ListFingerprints(limit=20)` 限制：点击长尾 flag 时下方可能无匹配行，
  属预期（§10.3 的 `?fp=` 单指纹下钻会补齐这条链路）。
- `/healthz` 目前随封禁一起 404（外部监控可能误报站点不可用）；`spa.go` 内 `/app/` 的
  403 分支为不可达死代码 —— 两项待处理，见 `docs/ip-guard-scoring.md` §7bis。