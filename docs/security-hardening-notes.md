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
| P2 | 算法升级（pHash / MinHash+LSH / 熵权 / 稳定性 / GeoIP） | ✅ 完成（2 处按实测调整：GeoIP 数据集换 DBIP 系 + maxminddb 读取层；组件集合改前端上报原始清单，见下） | 见 git log `P2-1` / `P2` |
| P3 | TLS + JA4 协议指纹 | ✅ 完成（2 处按实测调整：JA4 自实现不引 ja4plus；映射表为 FoxIO 样本数据，curl 走自有观测路径，见下） | 见 git log  |
| P4 | 平台证明与高级项 | 🔶 批一完成：ALTCHA PoW 全链路 + Play Integrity 服务端（mock 三用例 + 降级路径活体）；批二待办：WebAuthn passkey / 图聚类 / 行为采集 / 时钟偏移 | 见 git log  |
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

## P2 — 算法升级 ✅

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

### P2-2 MinHash + LSH 组件集合关联 ✅

- 域包（P2-1 已交付）：k=128 = 16 带 × 8 行，模 p=2^61−1，splitmix64 固定系数跨进程一致。
- **接线（本轮）**：前端 `fingerprint.js` 新增可选 `sets` 字段（fonts 实测清单 / webgl_exts /
  plugins，各 ≤64 项、单项 ≤64 字节，服务端 `sanitizeSets` 清洗）——组件分量的哈希值撑不起
  集合相似度（全变），规格书"来自现有 components"的前提不成立，改为上报原始清单。
- **签名改服务端计算**：`computeMinHashSig`（元素 = 键名前缀+值，防跨清单同名互撞；hex2048）。
  payload 的 `minhash_sig` 客户端字段**弃用不采信**（防伪造签名投毒 LSH 桶），有单测锁死。
- 引擎 `linkByMinHash`：签名写 16 桶（先删后插，签名变化旧桶自动失效）→ 同带召回（≤200）→
  候选取旧签名精确 `JaccardEstimate` > 0.8 → 写 `fp_links(kind=minhash, weight=Jaccard)` +
  0 分事件 `fp-minhash-link`。端口新增 `UpsertLSHBands` / `ListLSHCandidates` / `ListMinHashSigs`。
- 活体（8792）：同清单双指纹 → `minhash` 边 weight=1.0、事件「组件集合关联 1 个旧指纹
  （Jaccard 1.00）」、每指纹 16 桶；不同清单 → 零关联（单测）。

### P2-3 熵值加权 ✅

- 域包 `fpmath/entropy.go`：`EntropyWeights([]map[string]string) []float64`——每键按"有该键的
  行数"为分母，w=−log2(c(v)/N_key)，bits=Σw；全同值 0、缺失键不参与、空表 0。
- 每日 cron：`IPGuard.RefreshEntropyBits`（近 30 天、≤20000 行）写回 `entropy_bits`；
  `ENTROPY_CRON` 独立调度（默认 `30 4 * * *`），与 HOT_CRON 互不干扰。
- 接入：**三层积分系数** `min(1, entropy_bits/40)`（`EntropyFactor`，熵权未算=1 行为不变），
  在 `fpReportAPI` 对 env-flag 事件分四舍五入缩放——大众配置只计分不硬封，罕见组合足额。
  `fp-linked`/`fp-churn` 等二层事件保持原分值（已有互证护栏，不在本系数范围）。
- 单测：fpmath 熵权 2 例；httpapi 衰减（bits=20 → headless-ua 50→25 分）、无熵权足额对照、
  cron 刷新分布。活体：cron 触发后 7 条指纹 entropy_bits 落库（唯一值 4.39 / 共享分量被稀释 3.39）。

### P2-4 时间稳定性 EWMA + 换脸轮换检测 ✅

- 每次上报：与旧档案逐分量比，X=1 未变/0 变了，S=0.3·X+0.7·S_prev（`computeStability`），
  随 upsert 落 `stability`（均值）与 `comp_stability`（JSON）；首次出现的分量无历史不产生观测。
- 轮换检测 `checkRotation`：pHash/MinHash 关联到的旧指纹（last_seen ≥ 1h）上，设备级稳定分量
  `fonts/webgl/renderer` 历史稳定（S ≥ 0.7）却在当前指纹取值不同 → 换脸实锤
  `env-flag:fpb_rotation_detected`（+25，灰度 0 分）。正常驱动漂移只动 canvas，不会连字体
  清单与显卡型号一起换——以此区分"同设备漂移"与"刻意轮换"。
- 单测：稳定历史 + 关联 + 突变 → 命中（灰度 0 分 / 关灰度 25 分、明细含分量名）；稳定度不足
  不判；活体：二次上报 S=0.3、comp_stability {canvas:0.3,fonts:0.3}。

### P2-5 GeoIP 交叉核验 ✅（数据集按实测调整）

- **数据源调整**：规格书指定的 `geo-whois-asn-country` 数据集已被上游下线（2026-06 起
  ip-location-db 改 GitHub Releases 分发、WHOIS 数据合规下架）→ 换同许可（CC BY 4.0）的
  **`dbip-country.mmdb` + `dbip-asn.mmdb`**；DBIP 署名要求记入 README。
- **读取层实测调整**：DBIP release 库的 databaseType 是 `country ipvAll`/`asn ipvAll`，
  geoip2-golang 类型化读取器直接拒绝（"reader does not support"）→ 改用底层
  **maxminddb-golang** 宽容解码（双 schema：顶层 `country_code` 兼容 GeoLite2
  `country.iso_code`），geoip2 依赖随之移除。
- `internal/infrastructure/geoip`：缺失即禁用（离线部署不报错）；`HostingOrg` 关键词表
  （宁可漏判不误判，消费级 ISP 用精确字样区分）；国家→大洲映射表（跨洲国家宽松集合）+
  IANA 时区大洲判定，**只有跨洲才计分**。
- CLI `githubhot geo download`：safehttp 通道、原子写、体积下限护栏（拒绝疑似上游变更）。
- 引擎 `geoEnrich`：上报时补全 `ip_profiles` 的 asn/asn_type/geo_country/geo_tz
  （**upsert 建档**——第一层档案延迟刷库，只 UPDATE 会丢数据，实测发现后修正）；两项检测
  灰度 0 分：`fpb_tz_geo_mismatch`（+10 出灰度）、`fpb_hosting_mobile_ua`（+15 出灰度）。
- 活体：223.5.5.5（AS37963 阿里）+ America/New_York → 双命中、明细含 ASN 与国家；
  对照（同 ASN + Asia/Shanghai、无移动 UA）→ 零误报；`ip_profiles` 三行 geo 字段齐全。

### 两笔遗留小账（§7bis）✅

- `/healthz` 封禁豁免：存活探针始终如实应答（封禁 IP 也是 200），外部监控不再把"已封禁"
  误报成"站点宕机"；其余路径维持全站 404。单测 + 活体（登录→手动封禁→404/200→解封恢复）。
- `spa.go` `/app/` 的 403 分支确认为不可达死代码（IPGuard 中间件先 404）→ 删除，限频逻辑保留。

---

## P3 — TLS + JA4 协议指纹 ✅

### P3-1 上 TLS（三模式）✅

- `internal/interfaces/cli/serve.go` 重写为 `serveHTTP`：**证书 TLS**（`TLS_CERT`/`TLS_KEY`）、
  **ACME 自动签发**（`ACME_DOMAIN` 逗号分隔，`data/acme` 缓存，80 端口自动监听 HTTP-01 挑战）、
  **纯 HTTP**（两者都无 = 本地开发模式，JA4 随之优雅关闭）。`REDIRECT_HTTP=1` → 80 端口 301
  跳 HTTPS（目标含非 443 主端口，沙盒 8792 亦可跳转）；80 绑定失败仅告警不阻断主服务。
- HSTS 中间件（`max-age=31536000; includeSubDomains`）仅 TLS 模式启用（`Server.TLSMode`）；
  `gh_id` cookie 改 `Secure: r.TLS != nil || X-Forwarded-Proto == "https"`（与管理端会话
  cookie 同口径）。
- 活体：`https://127.0.0.1:8792/healthz` 200 + HSTS 头在位；`http://127.0.0.1/api/v1/hot`
  → `301 → https://127.0.0.1:8792/api/v1/hot`；`gh_id` Set-Cookie 带 `Secure`（自签证书直连 TLS）。

### P3-2 JA4 采集 ✅（自实现，零第三方依赖）

- 规格授权路径复核：`exaring/ja4plus` 与 FoxIO 系许可纠缠 → 采用规格书预置的 **stdlib 降级
  方案**：`tls.Config.GetConfigForClient(*tls.ClientHelloInfo)` 捕获 ClientHello（含原始
  Extensions 顺序表），**按 FoxIO 公开规格自实现 JA4**（JA4 本体 BSD-3，规格书 §0.8 允许）。
- 域包 `internal/domain/tlsfp/ja4.go`（纯函数）：归一化再哈希——cipher/extension 排序、
  GREASE（0x?a?a）全域剔除、扩展哈希剔除 SNI(0000)/ALPN(0010)、签名算法**保序**追加、
  ALPN 取首值首尾字母数字（非字母数字回退十六进制，规格示例 0x30 0x31 0xab 0xcd → "3d"）、
  计数 99 封顶、版本取 supported_versions 最大值映射（0x0304→"13"…）。
- **单测含官方向量**：规格书 §2 的 15 套件排序串 → 哈希 `8daaf6152771` 逐位一致。
- 捕获装配（`ja4Collector`）：实测发现 Go 的 TLS 握手是**惰性**的——`http.Server.ConnContext`
  在 accept 时先于握手执行，此时指纹还不存在；改为上下文注入**延迟解析引用**
  （`JA4Resolver.ResolveJA4()`，请求时刻握手已完成），`ConnState(StateClosed)` 清理条目。
  关联键：`hello.Conn`（裸连接）与 `tls.Conn.NetConn()` 对齐。
- 存储：`ip_fingerprints.ja4` 列（迁移），随 fp/report 落库；纯 HTTP 部署为空。

### P3-3 UA↔TLS 交叉核验 ✅（含实测调整）

- `internal/infrastructure/ja4db`：加载 FoxIO `ja4plus-mapping.csv`（`githubhot ja4 update`
  拉取到 `DATA_DIR/ja4-mapping.csv`；**实测该 CSV 在仓库根而非 technical_details/CSVs/**）。
  解析容错：按表头定位 JA4/应用列 + `FieldsPerRecord=-1`（容忍手工编辑的行缺列——
  活体中手工追加行少一个逗号曾导致整表加载失败、核验静默跳过，已修死）。
- 规则（保守）：JA4 ∈ 已知非浏览器栈（curl/Go/Python/okhttp… 关键词表）且 UA 声称浏览器 →
  `fpb_ua_tls_mismatch` **+25**（灰度 0 分）；JA4 不在库 → `tls_unknown` 仅记录；
  映射未加载/纯 HTTP → 整体降级跳过。
- **实测调整**：FoxIO 的映射表是样本数据（23 条 JA4，curl 条目是 JA4H 而非 JA4，Go stdlib
  未收录）→ curl 的误报验证采用"运营者自有观察"路径：活体采集到的 curl 真实指纹
  （`t13i2011h1_2b729b4bf6f3_36bf25f296df`，来源即真实 curl）追加进映射表后命中。
  后续项：Go stdlib 自检指纹（服务端自探测后写映射）可让最常见 Go 爬虫开箱即命中。

### P3-4 验收 ✅

| 项 | 结果 |
|---|---|
| curl + Chrome UA → `fpb_ua_tls_mismatch` | ✅ 命中（0 分灰度，明细含映射应用名与 JA4） |
| 真实浏览器零误报 | ✅ Edge(headless) 活体：JA4 `t13i1515h2_8daaf6152771_d8a2da3f94cd` 捕获入库，**cipher 哈希段与 FoxIO 规格 Chrome 向量逐位一致**；未在映射 → `tls_unknown` 0 分不判 mismatch；规则方向另有单测（`TestUATLSBrowserOK`） |
| HTTP→HTTPS 301 | ✅ `301 → https://127.0.0.1:8792/...`（含非 443 端口） |
| `gh_id` 带 Secure | ✅ Set-Cookie 实测含 `Secure` |
| ACME 真签发 | ⏳ 需公网域名与 80/443 可达，沙盒不可测；装配路径已实现（autocert + 挑战路由 + 缓存），上生产时验证 |

单测：tlsfp 8 例（含官方向量）、ja4db 4 例、httpapi UA↔TLS 4 例（mismatch 灰度/出灰度、
浏览器 OK、tls_unknown、双降级）；`go test ./...` 18 包全绿、`go vet` 干净、build 通过。

---

## P4 — 平台证明与高级项（批一：ALTCHA + Play Integrity 服务端 ✅）

### P4-3 ALTCHA 工作量证明 ✅

- 域包 `internal/domain/altcha`（自研 ~120 行，无依赖）：challenge = base64url(`expiry:difficulty:salt:hmac`)，
  hmac = HMAC(secret, salt+expiry)——**payload 内为绝对过期时间戳**（初版用纯时长没有校验锚点，
  活体前单测即暴露，已改）；客户端暴力 nonce 使 SHA-256(challenge+nonce) 前导零比特 ≥ difficulty。
- **实测调整（签名语义）**：规格书 "signature=HMAC(fp+gh_id)" 若由客户端计算则无密钥可言
  （攻击者可自造 fp+签名，绑定形同虚设）→ 改为**服务端签发时计算**（挑战端点带 ?fp=，
  HMAC 封入 challenge+fp+gh_id，客户端原样回传）——同样实现"防跨指纹重放"且更强，
  有单测锁死（伪造签名/跨指纹重放均拒）。
- 端点：GET /api/v1/altcha/challenge?fp=、POST /api/v1/altcha/verify；fp/report 挂钩：
  `ALTCHA_SECRET` 在位且 `ALTCHA_DIFFICULTY>0` 时强制——裸上报 401 + `altcha-missing` 10 分
  弱证据；**DIFFICULTY=0（默认）时行为与 PoW 之前完全一致**（灰度纪律：先发前端 altcha.js
  自动求解，观察后再置 12 启用，与规格"默认 12"的偏离已记录）。
- 前端 `web/src/lib/altcha.js`：领挑战 → crypto.subtle 暴力求解（difficulty 12 实测 31 次命中
  平均量级）→ fp/report 自动携带；服务端未启用返回 null 不带字段。npm test 40 例全绿
  （含 altcha 3 例：难度满足、确定性、超时保护）。
- 活体（8792 TLS 实例）：裸上报 **401 + 10 分计分** ✓；带 PoW HTTP 200（rtt **8ms**，≪50ms 预算）✓；
  同解换指纹重放 → 401「signature 与指纹不匹配」✓；关闭（DIFFICULTY=0）→ 裸上报 200 行为不变 ✓。

### P4-1 Play Integrity 服务端 ✅（Android 侧行为契约见规格 §14.8，独立仓库同步）

- 域包 `internal/domain/attest`：nonce 内存消费型存储（32B hex、TTL 10min、绑 gh_id、
  SHA-256 键防重放）；Google 判定解析评估（纯函数，**规格三用例 mock 单测**：
  PLAY_RECOGNIZED / UNRECOGNIZED_VERSION（自分发预期，不是拒绝条件）/ requestHash 不符，
  另含证书集、包名、设备档位满档/半分、10 分钟新鲜度）；降级路径评估
  （APP 上报签名证书 + ThreatDetect，威胁标记即拒）。
- 基础设施 `internal/infrastructure/playintegrity`：GCP service account → **自签 RS256 JWT**
  （stdlib crypto/rsa，无新依赖）→ oauth2 换 token（带过期缓存）→
  decodeIntegrityToken；出站全走 safehttp。
- 端点：GET /api/v1/app/attest/challenge（nonce 绑 gh_id）；POST /api/v1/app/attest/verify
  （主路径 {token} / 降级 {cert_sha256, threat} → {level, valid, reason, attest_required}）。
  结果 JSON 写回 `ip_fingerprints.attestation`（新列，绑定 X-Device-Fp 指纹；需先有 fp/report
  归档行）；无效记 `attest-failed` 10 分弱证据；`ATTEST_REQUIRED=1` 响应携带只读降级标志
  （APP 侧行为契约）。
- 活体（降级路径，Google 主路径需真 SA + GMS 真机为手动验收项）：challenge 签发 ✓；
  证书在期望集 → signature_fallback 通过 ✓；证书不符 → valid=false ✓；**nonce 消费后重放
  → 拒绝** ✓；attestation 列落库 ✓。

### P4-5 行为生物特征采集（规则版）✅ + P4-6 时钟偏移 ✅

- 前端 \`web/src/lib/behavior.js\`：mousemove 节流 16ms 滑窗 500 → 速度均值/方差、曲率均值、
  jerk（加加速度）方差、方向变化率；keydown/keyup 滑窗 100 → dwell/flight 均值方差；
  时钟偏移：每 60s 采样 \`Date.now() - performance.now()\`（仅页面可见，后台节流假点规避），
  20 点最小二乘斜率 × 1e6 = ppm。纯函数（node --test 5 例：机械匀速直线 → 方差/曲率 0、
  真实抖动轨迹非零、等长按键 dwell 方差 0、固定漂移 → 50000ppm）。
- 上报：每 5 分钟或页面隐藏/卸载（fetch keepalive）→ fp/report 增量携带 \`behavior\` +
  \`clock_skew_ppm\`（可选字段；payload 字段用 json.RawMessage——活体中前端发对象、
  服务端 string 字段曾致 400，已修）。
- 服务端规则：清洗（\`behavior.Parse\`：事件数上限、NaN/Inf 拒绝）→ \`MachineSignals\`：
  有数据前提下（鼠标 ≥20 事件 / 击键 ≥10）速度方差 0 / 曲率均值 0 / dwell 方差 0 →
  \`behavior_machine\` +15（灰度 0 分）。活体：机械匀速+机械击键 → 命中（0 分）；
  人类行为对照（方差非零）→ 零误报 ✓。落库：behavior/clock_skew_ppm 列实测写入 ✓。
  隐私边界：只传时序统计量，不含坐标原值与按键内容。

### P4-4 图聚类（连通分量版）✅

- 域包 \`internal/domain/fpcluster\`：union-find 连通分量，四类边——①共享 IP ≥2；
  ②fp_links 既有关联边（pHash/MinHash/相似/物理/时间共现，P2 产物）；③时钟偏移差
  <5ppm 且 behavior 余弦 >0.9（双条件）；④JA4 相同且 UA 互异。单测 5 组（共享 IP 阈值、
  传递闭包合并、双条件缺一不连、JA4+UA 异、孤立安全）。
- 引擎 \`RefreshClusters\`：30 天窗口全量构图 → union-find → 整表重写 \`fp_clusters\` +
  回填 \`cluster_id\`（事务）。cron：与熵权刷新同一每日调度（ENTROPY_CRON，默认 04:30）。
- 管理端：summary additive 字段 \`clusters\`（Top-10，含 id/members/size/reason，
  json 标签小写）；§10.4 的完整集群视图（Tab + 力导向图）留待前端整合冲刺。
- 活体：cron 实跑 → 9 个测试指纹经"JA4同"边聚成 1 簇（同一 TLS 栈 + 互异 UA——
  正是规则 4 的目标形态），cluster_id 全员回填，summary 可见 ✓。
- **待办（记录）**：规格的"簇内成员被 ban → 连坐系数 ×1.5"尚未接入——现 fp-linked
  70 分已顶到强类上限，×1.5 会越过单类封顶设计；拟以独立弱证据 kind（cluster-linked）
  形式接入并与 iprisk 对齐，留待下批与 P5 review 队列一起做。

### P4-2 WebAuthn 通行密钥（管理端免密登录）✅

- 依赖：go-webauthn v0.18.2（FIDO2，纯 Go）。存储：`admin_credentials` 表
  （credential_id BLOB UNIQUE / public_key / attestation_type / sign_count / transports）。
- 端点（/api/v1/admin/passkey/*）：begin-register / finish-register（守卫组内，注册绑
  管理会话）；begin-login / finish-login（守卫外免密入口，成功后**复用 gh_admin_session
  会话通道**并按 P0-2 口径绑定 IP）；credentials 列表 / 删除。登录会话一次性 token
  （5 分钟 TTL，消费即失效）。
- env：WEBAUTHN_ENABLED=1 + WEBAUTHN_RP_ID（=域名）+ WEBAUTHN_ORIGIN（=完整 origin，
  **origin 的域必须与 RP ID 一致**——用 127.0.0.1 访问而 RP=localhost 会被浏览器拒，
  沙盒实测踩到后改用 https://localhost:8792）；WEBAUTHN_ONLY=1 关闭密码登录。
- 陷阱实录（每个都花了一轮排查，供后续实现者避坑）：
  1. **Go 的 TLS 握手惰性**（P3 同款）：ConnContext 时 body 未读——本处无影响；
  2. **finish-login 的 body 被 FinishLogin 二次消费**：本地解码后必须把 body 回填
     （r.Body = NopCloser(NewReader(raw))），否则 Parse error for Assertion（EOF）；
  3. **线格式全 base64url**（URLEncodedBase64 自定义解码，拒绝标准 base64 的 +/=）；
     断言体的 id/rawId/type 在**顶层**而非 response 内（与 WebAuthn 标准形状一致）；
  4. RP ID 必须是 origin 的可匹配域（localhost ↔ 127.0.0.1 不匹配）。
- 前端：AdminPasskeys.vue 管理页（列表/注册/删除）+ AdminLogin.vue 免密按钮
  （HEAD 探测 passkey 端点存在才显示；WebAuthn 不可用的浏览器自动隐藏）+ 路由 /admin/passkeys。
- 验收（playwright + CDP 虚拟认证器，headless Edge）：
  A. 密码登录 → 注册本设备密钥（认证器自动完成用户验证）→ 凭据落库 ✓；
  B. 清 Cookie → 免密登录 → 断言通过、会话建立 ✓；
  C. "错设备"（另一空虚拟认证器，无已注册密钥）→ 免密登录失败、错误展示 ✓
     （错误为 NotAllowedError 超时——认证器无匹配密钥时浏览器快速拒绝，
      服务端 finish-login 未被调用，攻击者无法探测会话状态）。
  存活验证：管理端会话签发后 admin/usage 正常访问 ✓。

---

## P5 — 行为机器学习（批一：标注体系 + iForest + LR 推理 ✅）

### P5-1 标注体系与数据管道 ✅

- DDL：`fp_labels` 表（fp+source 主键；label human/bot/uncertain；source admin/rule/model；
  confidence）+ `ip_fingerprints.anomaly_score` 列。
- 金标签：`POST /api/v1/admin/fp/label`（守卫组内，source=admin confidence=1.0，覆盖弱标签；
  非法 label 400）。弱标签：`RefreshRuleLabels` 每日 cron（与熵权/聚类同调度）——曾封禁 →
  bot(0.7)；灰度期命中 ≥2 项 fpb_*/botd_* → bot(0.6)；近 30 天零 flag 且有行为数据 →
  human(0.8)；uncertain 不落表。重建前清 source=rule（admin 金标签保留）。
- 导出：`githubhot ml export --out data/ml/behavior.jsonl`——16 维特征 map（键名与
  train_behavior.FEATURE_NAMES 严格一致），label/weight 取每 fp 最高置信标注，
  uncertain 行不导出。活体：金标签 2 条（human+bot）→ 导出 human 1 / bot 1、weight=1.0 ✓。
- 数据/gitignore：data/ml/、data/models/、scripts/ml/.venv/ 已忽略。

### P5-2 Isolation Forest ✅

- 自研纯 Go `internal/domain/fpmath/iforest.go`（~120 行零依赖，固定种子确定性）：
  100 树 × 256 子采样，c(n) = 2·H(n−1) − 2(n−1)/n（初版公式把 (n−1)/n 写成乘法，
  单测"内群 vs 极端离群"暴露后修正）。分数 = 2^(−E[h]/c(n)) ∈ (0,1)。
- 引擎 `RefreshAnomalyScores`（每日 cron）：近 7 天指纹 → 16 维特征向量 → 训练+打分 →
  anomaly_score 列；99.5 分位仅记录、99.9 分位记 anomaly-p999 0 分事件（进 review 关注）。
  样本 <10 跳过。单测：离群分显著高于内群、同种子确定性、分位阈值。

### P5-3 LR 推理 ✅（shadow 纪律全链路）

- `internal/domain/fpmath/lr.go`：标准化 + 点积 + sigmoid（~30 行零依赖）。单测手算对拍
  1e-9 精度（σ=0 防除零、未激活/维度不符返回 -1 哨兵）。
- 热加载：`lrLoader` 按 `ML_MODEL_DIR/behavior_lr_v1.json` 的 mtime 变化惰性重读
  （读失败保留旧模型）。fp/report 时 `scoreBehaviorML`：模型 **gate.pass 且 AUC≥0.85**
  才生效——先 shadow（0 分记录 behavior-ml-score）；出 shadow 后 score ≥0.8 才 20 分
  （与 P6-6 融合规则一致）。
- `githubhot ml check`：模型门槛透传校验（gate 未达标/文件缺失 → exit 1；达标 → exit 0），
  部署流水线可用作启用前置检查。活体：缺失/未达标/达标三态 exit 码实测 ✓。

### 记录待办

- P5-4 GBDT（treemodel.go）：条件触发项（LR AUC<0.85 或标注 >5 万）——沙盒
  export_lgbm_json.py 已就绪，Go 树遍历留待触发条件满足时实现。
- P5-6 PSI 漂移监控 + ML 诊断页：随 §10 管理端可视化整合交付。
- 弱标签"近 30 天 IP 数/会话时长"特征由导出端动态计算（session_minutes 用
  first_seen−last_seen 近似），行为特征需 fp/report 已携带 behavior（P4-5 前置）。

---

## P6 — 图算法升级（批一：Louvain + MIDAS + 图快照/GNN 写回管道 ✅）

### P6-2 Louvain 社区发现 ✅

- \`internal/domain/fpgraph\`：gonum v0.17 \`community.Modularize\`（Louvain，纯 Go），
  \`simple.NewWeightedUndirectedGraph\` 加权无向图。输入：fp 节点全量 + 加权边（weight=关联强度）。
- **验收单测（规格原文）**：双团伙 + 单桥接节点 → Louvain 切成两团而连通分量只有一团 ✓。
  另含：稠密三角 1 社区 ✓、无边图各自成社区 ✓。
- 引擎集成：\`RefreshClusters\` 改用 Louvain（分辨率 1.0），社团划分覆盖 cluster_id。
  保留连通分量域包（fpcluster）作为无权对照。risk_score = ban 占比 × min(1, size/10)
  引擎侧尚未接入（待 P6-6 融合批次）。

### P6-4 MIDAS 流式边异常 ✅

- \`internal/domain/midas\`（纯 Go，~100 行零依赖）：双 count-min sketch（当前片/历史累计），
  卡方型统计量 (a−s)²/max(1,s)，只计正偏差（负偏差 = 正常回访）。时间片 60s，深度 3，
  宽度 256 桶（宽度决定灵敏度：越大越不敏感）。冷启动：第一片跳过评分（无基线不假阳性）。
- **实现坑实录**：初版把冷启动检查放在 sketch 递增之前——第一片数据完全丢失、total 恒 0，
  第二片全报高分。修正为先递增再判冷启动。另修 sliceStart 需从首条观测校准（New 里
  time.Now() 会被测试注入的过去时间戳绕过，导致 rollSlice 永不触发）。
- 单测：协同攻击（20 IP × 20 fp 基线片 + 爆发片 → score 64 ≫ 3）✓；平稳流量 → 0 ✓；
  同边重访 → 0 ✓；时间片滚动 ✓。
- 引擎接入：\`midasScore\` 字段 + \`midasCheck\` 在 IPGuard Middleware 的第一层速率记录旁
  调用（每次请求更新 ip→fp 边）；>3σ 记 midas-alert 0 分（shadow），>5σ 加计 10 分。
  纯内存（重启丢失，规格允许）。

### P6-1 图快照管道 ✅ + P6-3 GNN 写回管道 ✅

- \`githubhot ml export-graph --out data/ml/graph.jsonl\`：单快照 JSON 行——
  fp 节点（挂 16 维特征 + 标签）/ ip 节点（零特征连接子）/ ua 节点（预留）+ 
  member_of_ip / ua_of / similar（pHash/MinHash）/ physical 边。活体：27 节点 / 14 边 ✓。
- \`githubhot ml import-gnn <graph_gnn_v1.json>\`：GNN 推理结果写回——每 fp 写
  gnn_score（bot 概率）+ gnn_embedding（64 维 JSON）。活体验证通路 ✓。
- \`ip_fingerprints\` 新列 gnn_score / gnn_embedding。
- **GNN 训练管道**（沙盒 scripts/ml/）已交付（GraphSAGE → ONNX，mlserve 真模型 RSS 82.2MB）。
  主管道消费流：ml export-graph → 沙盒训练 → ml import-gnn 写回。冷启动 Louvain 均值
  域包方法已就绪（fpgraph.Communities 均值可计算）。

### P6-3b sidecar 集成 ✅ + P6-6 融合规则 ✅

- go-client 迁入主仓库 \`internal/infrastructure/sidecarclient\`（三态：未配置→ErrDisabled、
  失败→ErrSidecar、成功→ScoreResult，500ms 超时）。
- fp/report 异步调 sidecar（fire-and-forget goroutine，不阻塞上报路径）：构建子图
  （目标 fp + 关联边 ≤10），成功 → 写回 gnn_score/gnn_embedding；失败 → 回落 Louvain
  社区均值（冷启动路径）。
- env：GNN_SIDECAR_URL（默认空 = 纯离线模式）+ GNN_SIDECAR_TOKEN。
- 融合规则（P6-6）：\`fusionCheck\` 在 sidecar 打分后执行——双高（behavior_ml ≥0.8 +
  gnn ≥0.8）→ fusion-severe 50 分（三层 iprisk 多证据通道）；单高 → fusion-review
  0 分（运维确认后人工标注）。皮尔逊正交性检查 <0.6 由每日 cron ml diag 批量执行。

## 已知边界 / 后续项

- P0-4 的指纹列表仍受 `ListFingerprints(limit=20)` 限制：点击长尾 flag 时下方可能无匹配行，
  属预期（§10.3 的 `?fp=` 单指纹下钻会补齐这条链路）。
- P2 新检测（rotation/tz_geo/hosting_mobile）与 P1 同守灰度：`FP_SCORE_SHADOW` 默认 0 分，
  管理端「检测命中统计」观察 7 天假阳性 < 0.5% 后再出分。
- 熵值加权现按"指纹全量分量"统计；若后续接入 `webgl_exts`/`plugins` 明细（P2-2 sets），
  熵权区分度会进一步提升（当前 sets 只进 MinHash 签名不入库）。