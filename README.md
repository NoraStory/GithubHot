<div align="center">

# GithubHot

**GitHub 开源项目热点 × AI 资讯热点 × 国内热榜 —— 三热度追踪**

采集几个世界每天真正在发生的事：GitHub 上正在爆发的新项目，AI 圈正在刷屏的大事件，
和国内网民正在围观的热点——再把它们对上号。

[![License: MIT](https://img.shields.io/badge/license-MIT-176b75?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![CI](https://img.shields.io/badge/tests-passing-3fb950?style=flat-square)](.github/workflows/ci.yml)
[![SQLite](https://img.shields.io/badge/storage-SQLite-003B57?style=flat-square&logo=sqlite&logoColor=white)](#)

</div>

---

## 这是什么

GithubHot 是一个自己找热点、自己写日报的网站框架（Go 实现，DDD 分层）：

- **GitHub 线**：官方 Search API（近 7 天创建、star 起量）+ trending 每日榜双轨发现，
  按快照差分计算真实增长——300 star 的新项目比静态 30 万 star 的老项目更"热"；
- **AI 线**：RSS / Hacker News / 通用 JSON 接口 / 网页列表等完整信源框架，
  LLM 预筛 → 同一标准独立两次评分 → 过门槛才入选，写中文标题和摘要；
- **国内线**：`hot_board` 信源直连抓取国内热榜（百度 / 微博 / 网易新闻榜 / 腾讯新闻榜）
  与榜单型 RSS（IT之家 / 钛媒体 / 爱范儿 / 极客公园 / 异次元软件等），
  多源共振轻管道聚合热度——这条线不走 LLM 打分，LLM 只负责每天一篇中文综述
  （`/api/v1/hot/domestic`，可用设置项 `domestic_summary_enabled` 关闭）；
- **融合**：LLM 判断哪些资讯在报道哪些项目，互相印证的事件获得热度加成——
  这是"双重热点"真正的交汇点；
- **日报**：每天自动产出 Markdown 日报与三榜网页，服务器常驻、GitHub Actions 定时、手动跑均可。

灵感与示范信源来自 [KKKKhazix/AIHOT](https://github.com/KKKKhazix/AIHOT)（MIT），致谢。
本项目是其领域思想（信源分级、事件聚簇、48h 热度窗口）在 Go + DDD 下的全新实现。

## 快速开始

需要 Go 1.22+、Node 18+（构建 Vue3 前端）和任意 OpenAI 兼容的模型 API Key（DeepSeek / 智谱 / 通义 / OpenAI 均可）。

```bash
git clone https://github.com/NoraStory/GithubHot.git
cd GithubHot
cp .env.example .env        # 填入 LLM_API_KEY，建议同时填 GITHUB_TOKEN

# 构建前端（Vue3 + Vite，产物嵌入二进制）
cd web && npm ci && npm run build && cd ..
rm -rf internal/interfaces/webui/dist
cp -r web/dist internal/interfaces/webui/dist

go build -o githubhot ./cmd/githubhot

# 跑一轮完整流水线（采集 → 发现 → 精选 → 聚簇 → 融合 → 热度 → 日报）
./githubhot run

# 启动 API + 三榜页 + 内置定时（服务器常驻模式）
./githubhot serve           # http://localhost:8787
```

产物都在 `data/`：`githubhot.db`（SQLite）、`digests/YYYY-MM-DD.md`（日报）、`site/index.html`（三榜页）。

## 配置

| 环境变量 | 必填 | 说明 |
|---|---|---|
| `LLM_BASE_URL` | ✅ | OpenAI 兼容接口地址，如 `https://api.deepseek.com` |
| `LLM_API_KEY` | ✅ | 模型 API Key |
| `LLM_MODEL` | ✅ | 主模型（预筛/评分/写作/裁决） |
| `LLM_MODEL_B` | | 第二评分模型；不配则用主模型换温度再评 |
| `LLM_EMBED_MODEL` | | 向量模型；配置后聚簇候选用语义召回，否则退化为词面相似度 |
| `LLM_EMBED_BASE_URL` | | 向量接口地址；留空复用 `LLM_BASE_URL`（混搭服务商时指定） |
| `LLM_EMBED_API_KEY` | | 向量接口 Key；留空复用 `LLM_API_KEY` |
| `LLM_EMBED_DIMENSIONS` | | 向量输出维度（MRL）；0 = 服务商默认。Qwen3-Embedding-8B 最大 4096 |
| `LLM_EMBED_STYLE` | | 向量接口风格：`openai`（默认）/ `ark-multimodal`（火山方舟 doubao-embedding-vision 系列） |
| `LLM_THINKING` | | `disabled` 关闭推理模型深度思考（提速约 5 倍） |
| `LLM_BUDGET_TOKENS_PER_DAY` | | 每日 Token 预算，超限自动熔断 LLM 阶段（0=不熔断） |
| `LLM_PRICE_IN_PER_M` / `LLM_PRICE_OUT_PER_M` | | 每百万 token 单价，仅用于成本估算展示 |
| `NOTIFY_WEBHOOK_URL` | | 日报/失败告警 webhook；格式 `NOTIFY_WEBHOOK_FORMAT` = raw / feishu / wecom |
| `ADMIN_PASSWORD_HASH` | 管理端 | Argon2id 密码哈希，设置后管理端走密码登录 + 服务端会话（`githubhot admin hash [密码]` 生成） |
| `ADMIN_TOKEN` | 管理端备选 | 备选鉴权方式：管理接口需带 `X-Admin-Token` 头 |
| `GITHUB_TOKEN` | 建议 | 无 token 限 60 次/小时；配置后 5000 次/小时 |
| `GITHUB_PROXY` | 国内服务器 | 直连 github.com 失败时的镜像前缀（如 `https://gh-proxy.com`），采集请求自动重试（5xx/429/网络错误，最多 3 次） |
| `HTTPS_PROXY` | 国内服务器 | 系统代理；采集与 LLM 请求均遵循 |
| `IP_GUARD_ENABLED` | | 三层 IP 防护总开关（默认开启，`0` 关闭） |
| `TRUSTED_PROXY` | 反代部署 | 受信代理 CIDR（逗号分隔）。为空 = 不信任任何代理头（用 TCP 对端地址）；配了才采信 `X-Forwarded-For`/`X-Real-IP` 并取最右非受信地址。直连部署留空 |
| `IP_GUARD_LOCAL` | | 回环/内网白名单（默认开启便于本机调试；公网部署务必设为 `0`） |
| `IP_GUARD_SECRET` | | 防护身份令牌 HMAC 密钥；不设置则每次启动随机生成（重启后旧令牌失效） |
| `HANDSHAKE_RATE_PER_MIN` | | 握手通道 `/api/v1/site/config` 单 IP 每分钟上限（默认 30）：该端点免签且会触发指纹归档写库，超限 429 并记分 |
| `ADMIN_SESSION_IP_STRICT` | | `1` = 管理会话必须来源 IP 完全一致；默认 `0` 放宽到 IPv4 `/24`、IPv6 `/64`（会话被跨网段使用即注销 + 记 80 分） |
| `FP_SCORE_SHADOW` | | 新增检测 flag 灰度开关，默认 `1` = 只记录不计分（`fpb_*` / `botd_*` 先在管理端「检测命中统计」观察假阳性率，达标后置 `0` 才计分） |
| `GEOIP_DB_PATH` | GeoIP | 国家库路径（默认 `data/geo.mmdb`，`githubhot geo download` 拉取 ip-location-db，CC BY 4.0，部署说明需保留致谢）。文件缺失时地理核验整体降级：时区↔IP 归属国跨洲矛盾（`fpb_tz_geo_mismatch`）不判定 |
| `GEOIP_ASN_DB_PATH` | GeoIP | ASN 库路径（默认 `data/geo-asn.mmdb`）：机房出口识别（`ip_profiles.asn_type=hosting`）+ 移动端 UA 组合核验（`fpb_hosting_mobile_ua`）；两项灰度默认 0 分 |
| `ENTROPY_CRON` | 熵值加权 | 分量熵权每日刷新调度（cron 五段式，默认 `30 4 * * *`）。熵权 = 各分量值出现频率的信息量之和；三层违规分 × `min(1, bits/40)` 作为封禁触发系数——大众配置只计分不硬封，罕见组合足额计分 |
| `ALTCHA_SECRET` / `ALTCHA_DIFFICULTY` | ALTCHA PoW | `ALTCHA_SECRET` 配置后 `/api/v1/altcha/challenge` 可用；`ALTCHA_DIFFICULTY>0`（推荐 `12`，≈4096 次哈希毫秒级）时 fp/report **强制**携带有效 PoW——裸请求 401 + 弱证据计分；`0`（默认）= 仅挑战端点可用、行为不变。签名由服务端签发时绑定 fp，跨指纹重放无效 |
| `PLAY_INTEGRITY_PACKAGE` / `PLAY_INTEGRITY_SA_JSON` | Play Integrity | APP 包名 + GCP service account JSON 路径（scope playintegrity）。齐备 → `/api/v1/app/attest/verify` 走 Google 主路径；缺失 → 自动支持降级路径（APP 上报签名证书 SHA-256 + ThreatDetect，银标准） |
| `APP_EXPECTED_CERT_SHA256` / `ATTEST_REQUIRED` | Play Integrity | 期望的 APK 签名证书指纹（逗号分隔，主/降级路径都校验）；`ATTEST_REQUIRED=1` 时无效设备进入只读降级模式（响应携带标志，APP 侧行为）。判定结果存 `ip_fingerprints.attestation` |
| `TLS_CERT` / `TLS_KEY` | TLS/JA4 | 证书与私钥路径，同时配置 → TLS 模式：HSTS、`gh_id` Secure cookie、**JA4 TLS 指纹捕获**（握手期 ClientHello）。与 `ACME_DOMAIN` 二选一；两者都无 → 纯 HTTP（JA4 核验自动关闭） |
| `ACME_DOMAIN` | TLS/JA4 | ACME 自动签发域名（逗号分隔，需公网可达 80/443；证书缓存 `data/acme`）。80 端口自动监听 ACME HTTP-01 挑战 |
| `REDIRECT_HTTP` | TLS/JA4 | `1` = 监听 80 端口把 HTTP 301 到 HTTPS（目标含非 443 端口；ACME 模式无需此开关） |
| `githubhot ja4 update` | TLS/JA4 | 子命令：下载 FoxIO ja4plus-mapping.csv → `data/ja4-mapping.csv`（`DATA_DIR/ja4-mapping.csv`）。文件在位时启用 **UA↔TLS 交叉核验**：浏览器 UA + 已知非浏览器 TLS 栈（curl/Go/Python/okhttp 等）→ `fpb_ua_tls_mismatch`（高置信 +25，灰度 0 分）；JA4 不在库 → `tls_unknown` 仅记录。文件缺失 → 核验整体降级 |
| `APP_SIGN_SEED` | APP/签名 | APP 请求签名种子（`githubhot admin seed` 生成，32B base64）。**serve 模式必填**：未配置或仍为出厂默认 `gh-dev-seed-v1` 时拒绝启动。APP 侧构建期注入同值，不再经 `/api/v1/site/config` 下发 |
| `APP_SIGN_SEED_GRACE` | 过渡期 | 旧种子列表（逗号分隔）：存量 APP 未发版时兜底验签；过渡期内验签不通过只观察不计分，建议 ≤14 天后清空 |
| `DATA_DIR` | | 数据目录，默认 `./data` |
| `PORT` | | serve 端口，默认 `8787` |
| `HOT_CRON` | | serve 内置调度（cron 表达式，本地时区），默认 `30 7 * * *` |
| `PROBE_INTERVAL_HOURS` | | 健康探针轮询间隔（小时），默认 `6`：全部启用信源 + LLM 网关 / GitHub API / 音乐上游 / 背景对象存储 / 本地库 |
| `MUSIC_PLAYLIST` | | 音乐馆（`/music`）默认歌单 id；歌单经 `/api/v1/music/playlist` 代理加载 |
| `CORS_ORIGINS` | | 允许跨域的来源（逗号分隔），默认不开放 |

> 管理端鉴权三模式（按 `.env` 自动切换）：`ADMIN_PASSWORD_HASH`（推荐，Argon2id 哈希 +
> 服务端会话 Cookie）→ `ADMIN_TOKEN`（请求头）→ 均未配置则开发模式放行。
> 三种模式均不设时管理端无防护，**公网部署必须至少配置其一**。
>
> 安全设计：所有出站请求（含 LLM 端点）经过 SSRF 防护——仅允许 http/https，
> 拒绝 localhost、环回、私有与保留地址。因此自建内网推理端点不可用，请用公网服务。
>
> 代理环境（Clash TUN/fake-ip 等）：设置 `HTTPS_PROXY` 后，DNS 级校验由代理负责，
> 主机名与 IP 字面量校验仍然生效——否则 fake-ip 返回的 198.18/15 伪地址会被
> SSRF 防护当作保留地址拒绝。

## 前端（Vue3，AnZhiYu 复刻）

`web/` 是 Vue3 + vue-router + Vite 前端，**用户端与管理端分离**：

- 用户端：首页（全屏背景视频轮播 + 一言打字机 + 搜索 + 国内热点两栏区）、GitHub 榜、
  AI 榜、国内热榜（`/domestic`）、融合观察、事件详情（综述/成员/关联项目/热度走势）、
  搜索、期刊列表与详情、归档（全量分页加载）、音乐馆（`/music`，meting-js + APlayer，
  歌单经后端代理）、工具库（`/tools`，MCP 接入说明就地展开）、
  统计/相册/标签云/分类/友链/空调等 AnZhiYu 复刻页、关于；
- 管理端（`/admin/*`，密码鉴权登录门）：Token 用量、内容诊断、运行历史、信源管理
  （增删/试抓）、事件锁定、期刊、**健康探针**（`/admin/probes`：信源与端点 6 小时定时探测、历史记录、手动触发）、**IP 防护面板**（`/admin/ipguard`：三层防护概览、
  Top 访客筛选、按时间筛选、单 IP 下钻详情——档案/封禁记录/设备指纹（Canvas、WebGL、
  UA、字体、时区等逐项列出）/违规事件、手动封禁与解封）；
- AnZhiyu 复刻元素：霞鹜文楷字体、粉主题令牌、毛玻璃吸顶导航、卡片投影悬停、彩色旋转标题符、
  亮/暗主题切换、背景音乐播放器、返回顶部、页面过渡动画。

本地前端开发：`cd web && npm run dev`（代理 API 到 :8787）。

## 架构（DDD）

```
cmd/githubhot            组合根：装配一切
internal/domain          领域层（纯业务，零 IO 依赖）
  ├─ source              信源上下文：十类信源（rss / json_api / web_list / hacker_news /
  │                      github_search / github_trending / script / hot_board，
  │                      预留 x_account / wechat_oa）、分级、抓取间隔
  ├─ item                原始资料上下文：判重、精选状态机（预筛→双评分→写作）
  ├─ github              项目热点上下文：项目实体、快照、增长热度领域服务
  ├─ story               事件上下文：聚簇聚合根、独立来源热度（48h/24h半衰/融合加成）
  ├─ digest              日报上下文：按日聚合
  └─ prompts             精选标准 KnowHow（全部提示词原文，改标准不改代码）
internal/application     应用层：用例集 + 流水线编排（只依赖端口）
internal/infrastructure  基础设施层：SQLite 仓储、RSS/HN/网页列表/国内热榜抓取器、
                         GitHub 双轨、OpenAI 兼容网关、SSRF 防护 safehttp、
                         Markdown/网页渲染
internal/interfaces      接口层：CLI、REST API（/api/v1，APP 契约）、页面（三榜/国内热榜/
                         管理端）、三层 IP 防护中间件
```

依赖方向严格向内：`interfaces / infrastructure → application → domain`。
领域服务（两个热度公式、相似度、聚簇策略）与全部提示词可独立单测；
应用层用例在测试中以假网关跑通端到端（见 `internal/application/pipeline_test.go`）。

详细设计与公式推导见 [docs/architecture.md](docs/architecture.md)。

## 热度是怎么算的

**AI 资讯事件**（按事件算、不按文章算）：

```
H = Σ(每个独立来源 w(分级) × 0.5^(年龄h/24)) × 10 × 融合加成
```

同一媒体发十篇只算一次（信源 ID + 域名去重）；48 小时窗口；24 小时减半；
T1（官方一手）权重 1.0、T2（媒体个人）0.6；与 GitHub 项目互证的事件 ×1.25。

**GitHub 项目**（按增长算、不按存量算）：

```
H = 10 × log2(1 + 24h新增star) + trending排名加成 + 新爆加成
```

热度来自快照差分，重复抓取不虚增；trending 1-3 名 +6、4-10 名 +3、11-25 名 +1；
首次发现 24h 内且有增长 +5。

**国内热榜**（多源共振轻管道）：同一话题被百度/微博/网易/腾讯多个榜单与 RSS 源
同时报道才抬升热度，报道源越多、越新鲜排名越高——不走 LLM 打分，纯信号聚合；
采集失败的榜单按指数退避自动降频，避免单个源失效拖垮整条线。

## 定时运行的三种姿势

1. **服务器常驻（推荐）**：`./githubhot serve`，内置 cron 到点自动跑，见 [deploy/githubhot.service](deploy/githubhot.service)；
2. **GitHub Actions**：仓库 Secrets 配 `LLM_API_KEY`（可选 `GITHUB_TOKEN`），
   [daily-hot.yml](.github/workflows/daily-hot.yml) 每天定时跑一轮并把日报提交回仓库；
3. **手动**：`./githubhot run`，随时跑一轮。

## 给手机 APP 和第三方

REST API 即契约（`/api/v1/*`，JSON，CORS 白名单可配）：

| 端点 | 说明 |
|---|---|
| `GET /api/v1/hot/github` | GitHub 项目榜 |
| `GET /api/v1/hot/news` | AI 资讯榜 |
| `GET /api/v1/hot/domestic` | 国内热榜（15 条 + LLM 每日综述，`domestic_summary_enabled` 控制） |
| `GET /api/v1/hot/fusion` | 融合观察 |
| `GET /api/v1/hot` | 三榜合一 |
| `GET /api/v1/digest/latest` | 最新日报（`?format=raw` 取 Markdown） |
| `GET /api/v1/digest/{date}` | 指定日期日报 |
| `GET /api/v1/sources` | 信源清单与适配器状态 |
| `GET /api/v1/story/{id}` | 事件详情（成员/关联项目/热度历史） |
| `GET /api/v1/stories?page=&pageSize=` | 全量事件分页（归档，默认仅资讯事件） |
| `GET /api/v1/search?q=` | 站内搜索 |
| `GET /api/v1/music/playlist?id=&server=` | 音乐馆歌单代理 |
| `GET /api/v1/agent/hot.md` | 双榜 Markdown（Agent 消费） |
| `GET /llms.txt` | 站点说明（llms.txt 约定） |
| `POST /api/v1/admin/sources` 等 | 信源管理（增删/试抓/推送/事件锁定，密码或 Token 鉴权） |
| `GET /healthz` | 健康检查 |

Flutter / Kotlin / Swift 客户端直接消费以上端点；领域层也可经 gomobile 编译为移动端库复用。

## MCP / 脚本推送 / 精选校准

```bash
# MCP 服务器（stdio JSON-RPC）：接入 Claude 等 Agent 客户端
githubhot mcp        # 工具：hot_github / hot_news / hot_fusion / search / latest_digest

# 脚本推送（AIHOT 的 script 信源）：外部采集脚本直接写入
githubhot push --source script-push --url https://... --title 标题 --summary 摘要

# SelectBench 精选校准：用标注样本回测预筛提示词（精确率/召回率/F1）
githubhot bench --file data/gold.jsonl   # 每行 {"text":"...","label":"pass|drop"}
```

MCP 接入说明也可在前端工具库 `/tools` 页面就地展开查看。
信源抓取间隔**按产出自适应**：连续空手而归指数退避（上限 24h），有产出回落基准。
信源 config 设 `fulltext: "1"` 可抓取文章正文（供 LLM 写作参考）。
事件页 `/story/{id}` 展示综述、成员报道、关联项目与热度走势；控制台可**人工锁定事件**
（锁定后聚簇不再自动合并——AIHOT 同款保护）。

## 换成你的行业

- **换信源**：往 `sources` 表里加你的信源（RSS/JSON/网页列表/HN 关键词均可配置）；
- **换国内榜**：`hot_board` 信源的 `board` 配置支持 `baidu` / `weibo` / `netease` / `tencent`
  四个网页榜单及榜单型 RSS（配 RSS 地址即可），国内直连无需代理；
- **换精选标准**：改 `internal/domain/prompts/prompts.go`，评分门槛在 `source.Tier.ScoreThreshold()`；
- **换热度口径**：两个公式分别在 `internal/domain/story/hotness.go` 与 `internal/domain/github/project.go`，参数是命名常量。

## 扩展点

`x_account`（X 账号）与 `wechat_oa`（公众号）是预留信源种类：实现
`application.SourceFetcher` 接口并在 `fetcher.Registry.Register` 注册即可接入，
领域模型无需任何改动。

## 自动化运维（CI/CD，两套流水线）

| 仓库 | 触发 | 流水线 |
|---|---|---|
| `GithubHot`（本仓库） | push main（前后端任一变） | 构建 web + 编译 linux/windows 二进制 → SSH 部署到服务器（上传、解包 dist、`systemctl restart githubhot`、healthz 自检）→ 发 `repository_dispatch` 通知 APP 仓库 |
| `GithubHot-App` | push main / 收到后端部署通知 | gradle 构建 APK → 产物归档 → SSH 推到服务器 `apks/` 目录 |

所需 GitHub 配置（两个仓库的 Settings → Secrets and variables）：

- `DEPLOY_HOST` / `DEPLOY_USER` / `DEPLOY_KEY`（SSH 私钥）/ `DEPLOY_PORT`（可选，默认 22）—— 两套流水线共用
- `DEPLOY_DIR`（Repository variable，默认 `/opt/githubhot`）
- `APP_REPO_PAT`（仅后端仓库；fine-grained token，需对 APP 仓库有 Actions 写权限，用于跨仓库触发）
- 未配置 `DEPLOY_HOST` 时部署步骤自动跳过，只做构建验证

服务器一次性准备：

```bash
sudo mkdir -p /opt/githubhot/{apks,videos} && sudo chown -R $USER /opt/githubhot
sudo cp deploy/githubhot.service /etc/systemd/system/githubhot.service
sudo systemctl daemon-reload && sudo systemctl enable githubhot
# 密钥等敏感环境变量写入 override.conf：
sudo systemctl edit githubhot   # DOUBAO_API_KEY=... 等
```

**APP 自动更新闭环**（一次配置，永久生效）：

1. 后端环境变量 `APP_FORCE_UPGRADE_URL=/app/githubhot-latest.apk`；
2. `/app/*.apk` 由后端从 `APK_DIR`（默认可执行文件旁 `apks/`）直出；
3. APP 启动握手拿到 `force_upgrade_url` → 弹不可跳过的更新框 → DownloadManager 下载 → FileProvider 调起系统安装器；
4. 之后每次 APP 仓库 push，新 APK 自动覆盖 `githubhot-latest.apk`，所有旧客户端下次启动即收到强制更新。

取消强制更新：去掉 `APP_FORCE_UPGRADE_URL` 环境变量重启即可。

## 开发

```bash
go test ./...        # 全部测试（无需网络与 API Key）
go vet ./...
go build -o githubhot ./cmd/githubhot
```

## License

[MIT](LICENSE) © NoraStory · 信源示范与领域思想致谢 [AIHOT](https://github.com/KKKKhazix/AIHOT)
