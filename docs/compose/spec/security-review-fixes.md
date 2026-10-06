---
feature: security-review-fixes
status: delivered
updated: 2026-10-06
branch: main
commits: d1f82c8..b3268a3
---

# 安全评审修复：P0 + P1 + 高危 P2（13 项）


## Report

**What was built** — 13 项安全评审修复全部落地（2 P0 + 8 P1 + 3 高危 P2）：GNN sidecar 健康检查契约修复（/healthz）、管理端开放模式治理（TLS 部署拒绝启动 / 本地 CRITICAL 警告）、握手限流并发原子化、指纹/IP 档案入库事务化 + ips 截断、safehttp 拨号时 SSRF 校验（消除 DNS rebinding TOCTOU）、25 处请求上下文恢复、四处免登录端点请求体上限、前端 11 处模板 window 引用修复、日报 XSS 后端转义、/privacy 返回 SPA、七处后台 goroutine panic 隔离（goSafe）、MLEngine 状态独立锁、mlserve reload 失败保留旧引擎。工作区此前滞留的 MLEngine 重构等未提交工作一并合流（含使 HEAD 恢复可编译的 realIP/getKeys 补全）。

**Verification** — go build ./... PASS；go vet ./... PASS；go test ./... 25 包全绿（含新增 10 组回归测试：并发计数守恒、sqlite 并发事务、DNS rebinding 端到端 TOCTOU、超限 body 拒收、mdEscape 中和、goSafe panic 隔离、mlserve 坏 reload 保留旧引擎等）；npm run build 成功；mlserve unittest 15/15；运行时实测：/privacy 返回 200 text/html、20MB fp/report 400 快速拒收、开放模式 CRITICAL 警告出现、GNN sidecar healthy=true fail_count=0。独立评审（general-3）：spec 合规 13/13、无 critical；评审 2 major / 4 minor 已处置——fp/report [DEBUG] 日志与 getKeys 删除、RefreshIForestNow 改独立 context、safehttp 注释笔误、F2 测试补至 6 组合；"mlserve 游离于 repo 版本控制之外"为架构级事项，见 Journey log。

**Journey log** — 1) 历史 H1 修复（锁外过滤）引入并发绕过回归：性能优化把"检查+写回"拆出锁外对限流类原语是危险模式，重做时把 DB IO 留锁外、状态判定留锁内。2) 修复引入回归的根因是"单测各自 mock 正确路径"：/health 契约断裂两侧测试都绿，需真机集成冒烟兜底（本次以 healthy=true fail_count=0 实测闭环）。3) MLEngine setState 若直接复用 e.mu 会死锁（checkLRModel 持 e.mu 调用链），同类"读改写 + 调用方已持锁"场景应默认拆独立锁。4) mlserve 与 repo 内 sidecarclient 是跨目录契约（F1↔F13），mlserve 不在 repo 版本控制内存在版本漂移风险，建议后续将 mlserve 纳入仓库或以 commit 固定引用（待用户决策）。5) 上一批工作 HEAD 编译不过（realIP 未定义）即滞留工作区，多代理并行时先单独构建自己的包、全仓 build 放最后。

## [S1] Problem

2026-10-06 对本仓库的运行 BUG 评审确认 2 个 P0、8 个 P1、15 个 P2、12 个 P3 问题。
本轮修复其中的 13 项：P0 全部 + P1 全部 + 3 项有进程崩溃/全面失效风险的高危 P2。
其余 P2/P3 本轮不做（见 S3）。工作区另有 11 个文件的未提交修改（上一批 MLEngine
重构等，编译测试均通过），经用户确认**在未提交工作之上直接修，最终一起提交**。

## [S2] Design

### P0

**F1. GNN sidecar 健康检查契约断裂**
`internal/infrastructure/sidecarclient/client.go:137` 请求 `/health`，mlserve 只注册
`/healthz` → 健康检查永久 404 → 3 次后 `healthy=false` → GNN 打分全量静默失效。
修复：请求路径改为 `/healthz`。不改其他逻辑。

**F2. 管理端开放模式治理（策略已定：TLS 拒绝启动，本地仅警告）**
`httpapi/auth.go:46-63`：`ADMIN_PASSWORD_HASH` 与 `ADMIN_TOKEN` 双空时 `adminAuth`
直接放行全部 `/api/v1/admin/*`。
- 在 serve 启动路径（`internal/interfaces/cli/cli.go` 的 `Serve`，紧邻
  `CheckAppSignSeed` 校验处）新增判定函数（放 config 或 httpapi 包，便于单测）：
  - 双空 **且** `cfg.TLSEnabled()`（公网部署标志）→ 返回错误拒绝启动，错误信息
    指引用 `githubhot admin hash` 生成 `ADMIN_PASSWORD_HASH`；
  - 双空且纯 HTTP → `log.Printf` CRITICAL 级警告（含"管理端完全开放"字样），
    不阻断本地开发流。
- 判定函数带单测（三种配置组合 × TLS/非 TLS）。

### P1

**F3. handshakeAllow 并发竞态（H1 修复引入的回归）**
`httpapi/ipguard.go:893-934`：锁外过滤+整体写回导致并发计数丢失、限流可绕过。
修复：过滤、判定、追加回归**单次持锁内**完成；仅超限时的 `event()`（DB IO）保持
锁外——在锁内标记需要计分，解锁后调用。删除"优化版"注释，写明为何必须持锁。
回归测试：`TestHandshakeAllowConcurrent`——限流阈值调小，并发 100 请求同一 IP，
断言放行数 ≤ 阈值（计数守恒，无 race 检测下即可验证逻辑正确性）。

**F4. UpsertFingerprint / TouchIPProfile 无事务 + ips 无上限**
`persistence/sqlite/ip_guard.go:168-276`（UpsertFingerprint）、`ip_profile.go:15-31`
（TouchIPProfile）：SELECT→merge→UPDATE 无事务，并发丢失更新/INSERT 主键冲突。
- 两者均用 `BeginTx` 包裹读改写；UpsertFingerprint 的 ErrNoRows→INSERT 路径在
  事务内执行，主键冲突转为重读合并重试一次（或 `INSERT ... ON CONFLICT`，以实现
  简单且不破坏现有 merge 语义为准）。
- `ips` 数组加常量上限（64），超出截断保留最近追加的。
- 新建 sqlite 包测试文件（当前 0 测试）：`TestUpsertFingerprintConcurrent`——
  并发 10 goroutine 上报同一 fp，断言 hits 不丢失、无错误返回。

**F5. safehttp DNS rebinding TOCTOU**
`infrastructure/safehttp/safehttp.go:151-195`：预解析校验与实际拨号之间存在
DNS 二次解析窗口。修复：非代理模式下 `Do` 使用自定义 `http.Transport` 的
`DialContext`——连接时解析 host → 逐 IP 过公网校验 → 用**校验通过的 IP** 直接
拨号（消除二次解析）。TLS SNI 与 Host 头由 Transport 依据原始 URL host 生成，
不受拨号 IP 影响。代理模式行为不变（出口解析由代理负责）。保留现有
CheckRedirect 逐跳校验。
测试：现有 safehttp 测试必须全过；新增"拨号函数拒绝解析到私网的 host"单测
（httptest 环境可控场景）。

**F6. `s.ctx()` 丢弃请求上下文（24 处）**
`server.go:358` 返回 `context.Background()`，`middleware.Timeout(15s)` 对 DB 查询
形同虚设。修复：删除 `ctx()` 方法，全部调用点（server.go 11、admin.go 9、spa.go 3、
rss.go 1）改用 `r.Context()`；签名忽略 `*http.Request` 的 handler 同步改参数名。
机械替换 + 编译验证。

**F7. 未鉴权端点请求体大小限制**
四处无 `http.MaxBytesReader` 的免登录入口，统一套上：
- `fp/report`（`ipguard_api.go:216`）→ 256KB（components/sets 较大）；
- `admin/login`（`auth.go:232`）→ 4KB；
- `admin/passkey/finish-login`（`adminpasskey.go:197`）→ 1MB（WebAuthn assertion）；
- `security/devtools-detected`（`ipguard_api.go:506`）→ 4KB。
超限返回 400（与现有错误风格一致）。测试：fp/report 用 httptest 发超限 body
断言拒绝（复现 20MB 全量读入问题已消除）。

**F8. App.vue 模板内 `window.anzhiyu` 引用（11 处按钮点击无效）**
Vue3 模板白名单不含 `window`，编译为 `_ctx.window` → TypeError。修复：模板内
11 处（`App.vue:593,662,665,671,709,713,714,753-755,765`）改为调用 setup 内的
method（新增薄包装函数，script 内 `window.anzhiyu` 用法合法保留）。
验收：模板属性中不再出现 `window.anzhiyu`；`npm run build` 通过。

**F9. 日报存储型 XSS（后端源头转义）**
`render/markdown.go:123` `mdEscape` 只转义 `|` 和换行，外部抓取标题含
`<img onerror=...>` 直通前端 `v-html`。修复：`mdEscape` 追加 HTML 字符转义
（`& < > " '`，注意先转 `&`）；同步修正 `DigestDetail.vue:25` 的不实注释。
选后端方案（一处覆盖全部日报生成路径，不给前端加依赖）。
测试：新建 render 包测试，断言 mdEscape 输出不含原始 `<`。

**F10. `/privacy` 返回裸 JSON**
`httpapi/privacy.go`：页脚"隐私协议"全页跳转直达裸 JSON。修复：handler 改为
委托 `s.spa(w, r)` 返回 index.html，由前端路由渲染隐私页。
验收：实测 `GET /privacy` 返回 200 `text/html` 且含 SPA 挂载点。

### 高危 P2

**F11. 请求路径派生 goroutine 无 recover（panic 打死进程）**
五处：`ogimage.go:68`、`ipguard.go:991`、`probes.go:55`、`ml_engine.go:128`、
`sidecar.go:67`。修复：httpapi 包新增 `goSafe(name string, fn func())`（defer
recover + log.Printf 带 name 与 panic 值），五处替换。`goSafe` 带单测（panic 函数
不冒泡）。

**F12. MLEngine `setState` 数据竞态**
`ml_engine.go:195-197` `setState` 无锁写 `e.state`，与 `Health()` 持 RLock 读并发。
**陷阱**：`checkLRModel` 已持 `e.mu` 再调 `setState`，若 setState 直接加 `e.mu`
将死锁（Mutex 不可重入）。修复：`state` 改由独立 `stateMu sync.RWMutex` 保护；
`setState` 写锁；`Health`/`HealthStatus` 读 state 处改用 `stateMu.RLock`
（metrics 仍用 `e.mu`，两锁无嵌套交叉持有）。

**F13. mlserve `/admin/reload` 失败清空引擎**
`mlserve/app/main.py:125-128,199-205`：加载失败也把 `state["engine"]=None` 顶掉
旧引擎 → `/v1/score` 全量 503；并发 reload 无锁。修复：加载成功才原子替换
`state["engine"]`，失败保留旧引擎并返回错误详情；模块级 `threading.Lock` 串行化
reload。测试：mlserve 现有 unittest 全过 + 新增"坏模型文件 reload 后旧引擎仍在
位"用例。

## [S3] Out of Scope

- 其余 P2（realIP 信任链、NotFound→SPA 与 404 检测、fpLast/ghAvatarCache 增长、
  passkey nil panic、UpsertBan 幂等语义、LSHBands 事务、cluster 过期封禁计分、
  前端路由复用、onMounted 错误处理、ip/check 死端点、writeErr 透出细节）；
- 全部 P3；
- `LLM_API_KEY` 失效问题（环境配置，非代码）；
- `repo/.git` 无 remote，仅本地提交、无法推送（物理隔离设计，按用户全局指令提交，
  push 由用户在主仓库侧合并时处理）；
- 不引入任何新依赖（后端/前端均零新增）。

## Tasks

- [x] T1: sidecarclient 健康检查改 `/healthz` — acceptance: 代码中无 `+"/health"` 非 `/healthz` 请求路径；mlserve 真机健康检查返回 200（covers: F1）
- [x] T2: 开放模式判定函数 + serve 启动拦截/警告 — acceptance: 判定函数单测覆盖 6 种组合；TLS+双空时 Serve 返回错误，纯 HTTP+双空时日志含"管理端完全开放"（covers: F2）
- [x] T3: handshakeAllow 回归锁内判定 — acceptance: `TestHandshakeAllowConcurrent` 并发放行数 ≤ 阈值（covers: F3）
- [x] T4: UpsertFingerprint/TouchIPProfile 事务化 + ips 截断 — acceptance: `TestUpsertFingerprintConcurrent` 并发后 hits 守恒无错误；ips > 64 截断（covers: F4）
- [x] T5: safehttp DialContext 连接时校验 — acceptance: 现有 safehttp 测试全过 + 新增拨号校验单测过（covers: F5）
- [x] T6: s.ctx() → r.Context() 全量替换 — acceptance: `rg "s\.ctx\(\)"` 0 命中；build/vet/test 全过（covers: F6）
- [x] T7: 四处免登录端点 MaxBytesReader — acceptance: httptest 超限 body 断言拒绝的单测过（covers: F7）
- [x] T8: App.vue 模板 window.anzhiyu 11 处改 method — acceptance: 模板属性 0 命中 `window.anzhiyu`；`npm run build` 过（covers: F8）
- [x] T9: mdEscape 转义 HTML 字符 + 注释修正 — acceptance: render 新单测断言输出无原始 `<`；测试过（covers: F9）
- [x] T10: /privacy 委托 spa — acceptance: 实测 GET /privacy 200 text/html（covers: F10）
- [x] T11: goSafe helper + 五处 goroutine 替换 — acceptance: `TestGoSafe` panic 不冒泡；build 过（covers: F11）
- [x] T12: state 独立锁消除 setState 竞态 — acceptance: build/test 过；checkLRModel 与 Health 无锁交叉（covers: F12）
- [x] T13: mlserve reload 原子替换 + 锁 — acceptance: mlserve unittest 全过 + 新增失败保留旧引擎用例过（covers: F13）
- [x] T14: 全量验证 — acceptance: `go build/vet/test ./...`、`npm run build`、mlserve unittest 全绿；serve 实测 /privacy HTML、20MB 拒收、开放模式警告（covers: S2 全部；depends: T1-T13）
- [x] T15: 独立评审子代理审阅完整 diff — acceptance: spec 合规/正确性/一致性三结论，critical 清零（depends: T14）
