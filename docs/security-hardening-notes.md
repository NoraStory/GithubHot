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
| P1 | 指纹浏览器识别（六路采集 + BotD + 计分接入） | ⏳ 待办 | — |
| P2 | 算法升级（pHash / MinHash+LSH / 熵权 / 稳定性 / GeoIP） | ⏳ 待办 | — |
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

## 已知边界 / 后续项

- P0-4 的指纹列表仍受 `ListFingerprints(limit=20)` 限制：点击长尾 flag 时下方可能无匹配行，
  属预期（§10.3 的 `?fp=` 单指纹下钻会补齐这条链路）。
- `/healthz` 目前随封禁一起 404（外部监控可能误报站点不可用）；`spa.go` 内 `/app/` 的
  403 分支为不可达死代码 —— 两项待处理，见 `docs/ip-guard-scoring.md` §7bis。
