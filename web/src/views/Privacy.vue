<script setup>
import { onMounted } from 'vue'

onMounted(() => {
  document.title = '隐私政策 - GithubHot'
})
</script>

<template>
  <div id="page">
    <div id="page-header" class="full_page">
      <div id="site-info">
        <h1 id="site-title">隐私政策</h1>
        <div id="site-subtitle">
          <span id="subtitle">GithubHot 数据收集与隐私保护说明</span>
        </div>
      </div>
    </div>

    <main class="layout" id="content-inner">
      <div id="page" class="flink">
        <article class="post-content">
          <div class="privacy-policy-content">
            <section class="policy-section">
              <h2>📋 概述</h2>
              <p>
                GithubHot（以下简称"本站"）致力于保护用户隐私。本隐私政策说明我们如何收集、使用和保护您的信息，
                特别是与 IP 封禁系统和设备指纹识别相关的数据处理。
              </p>
              <p class="update-time">最后更新时间：2025-01-15</p>
            </section>

            <section class="policy-section">
              <h2>🔒 我们收集哪些信息</h2>
              
              <h3>1. 自动收集的信息</h3>
              <div class="info-card">
                <h4>网络信息</h4>
                <ul>
                  <li><strong>IP 地址</strong>：用于地理定位、频率限制和恶意访问检测</li>
                  <li><strong>User-Agent</strong>：浏览器类型、操作系统版本</li>
                  <li><strong>HTTP Referer</strong>：访问来源页面</li>
                  <li><strong>访问时间戳</strong>：请求时间和频率</li>
                </ul>
              </div>

              <div class="info-card">
                <h4>浏览器指纹（仅在前端检测到异常行为时收集）</h4>
                <ul>
                  <li><strong>Canvas/WebGL 指纹</strong>：GPU 渲染特征（不包含用户内容）</li>
                  <li><strong>字体列表</strong>：系统已安装字体（用于设备唯一性识别）</li>
                  <li><strong>屏幕属性</strong>：分辨率、色深、像素比</li>
                  <li><strong>时区与语言</strong>：系统时区和浏览器语言设置</li>
                  <li><strong>硬件并发数</strong>：CPU 核心数（navigator.hardwareConcurrency）</li>
                  <li><strong>WebRTC 候选地址</strong>（可选）：本地网络接口 IP（用户可在浏览器中禁用 WebRTC）</li>
                  <li><strong>音频指纹</strong>：AudioContext 输出特征（设备音频栈差异）</li>
                  <li><strong>插件与扩展特征</strong>：已安装的浏览器插件列表</li>
                </ul>
                <p class="note">
                  <strong>触发条件</strong>：仅在检测到以下行为时才会收集完整指纹：
                </p>
                <ul>
                  <li>短时间内大量请求（>60 req/min）</li>
                  <li>访问大量 404 页面（扫描行为）</li>
                  <li>使用自动化工具特征（Selenium、Puppeteer 等）</li>
                  <li>多次登录失败</li>
                </ul>
              </div>

              <div class="info-card">
                <h4>行为特征</h4>
                <ul>
                  <li><strong>访问路径</strong>：浏览的页面 URL（不包含查询参数中的敏感信息）</li>
                  <li><strong>请求频率</strong>：1 分钟/5 分钟内的请求次数</li>
                  <li><strong>404 错误率</strong>：无效路径访问频率</li>
                  <li><strong>DevTools 状态</strong>：前端检测到开发者工具打开（仅记录，不封禁）</li>
                  <li><strong>键盘输入模式</strong>：按键间隔时序、输入速度、节奏特征（用于识别自动化脚本，不记录具体按键内容）</li>
                  <li><strong>鼠标移动轨迹</strong>：移动坐标序列、点击位置、滚动行为（用于区分人类与机器人）</li>
                </ul>
                <p class="note">
                  <strong>重要说明</strong>：键盘和鼠标数据仅收集<strong>时序和模式特征</strong>，不记录具体内容：
                </p>
                <ul>
                  <li>键盘：仅记录按键间隔（如 [120ms, 85ms, 200ms]），不记录按了哪些键</li>
                  <li>鼠标：仅记录移动曲线和速度特征，不关联具体操作对象</li>
                  <li>目的：自动化工具的输入模式与人类显著不同（如固定间隔、直线移动）</li>
                </ul>
              </div>

              <h3>2. 我们不收集的信息</h3>
              <div class="info-card safe">
                <ul>
                  <li>❌ 键盘输入的具体内容（不知道您输入了什么文字、密码）</li>
                  <li>❌ 鼠标点击的具体元素内容</li>
                  <li>❌ 剪贴板内容</li>
                  <li>❌ 表单字段的具体值（搜索词、评论文本等）</li>
                  <li>❌ Cookie 中的身份认证令牌（仅哈希后存储）</li>
                  <li>❌ 本地存储（localStorage/sessionStorage）中的用户数据</li>
                </ul>
              </div>
            </section>

            <section class="policy-section">
              <h2>🎯 数据用途</h2>
              <div class="usage-grid">
                <div class="usage-card">
                  <h4>🛡️ 安全防护（主要用途）</h4>
                  <ul>
                    <li>识别和封禁恶意 IP 地址</li>
                    <li>检测爬虫和自动化工具</li>
                    <li>防止 DDoS 和暴力攻击</li>
                    <li>关联同一设备的多个 IP（防止 IP 切换绕过封禁）</li>
                  </ul>
                </div>
                <div class="usage-card">
                  <h4>📊 统计分析（匿名）</h4>
                  <ul>
                    <li>访问地区分布（国家/省份级别）</li>
                    <li>浏览器和操作系统占比</li>
                    <li>热门内容排行</li>
                    <li>流量趋势分析</li>
                  </ul>
                </div>
                <div class="usage-card">
                  <h4>🔧 故障排查</h4>
                  <ul>
                    <li>定位异常请求来源</li>
                    <li>分析误封禁案例</li>
                    <li>优化防护规则</li>
                  </ul>
                </div>
              </div>
            </section>

            <section class="policy-section">
              <h2>⏱️ 数据保留期限</h2>
              <table class="retention-table">
                <thead>
                  <tr>
                    <th>数据类型</th>
                    <th>保留期限</th>
                    <th>说明</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>违规事件记录</td>
                    <td>7 天</td>
                    <td>自动清理，用于短期封禁判定</td>
                  </tr>
                  <tr>
                    <td>设备指纹</td>
                    <td>30 天</td>
                    <td>未活跃则删除，活跃指纹持续更新</td>
                  </tr>
                  <tr>
                    <td>IP 封禁记录</td>
                    <td>封禁期结束后立即删除</td>
                    <td>通常 30 分钟到 24 小时</td>
                  </tr>
                  <tr>
                    <td>访问日志（聚合）</td>
                    <td>90 天</td>
                    <td>仅保留统计数据，不含原始请求</td>
                  </tr>
                  <tr>
                    <td>WebRTC 候选地址</td>
                    <td>24 小时</td>
                    <td>仅用于短期设备关联</td>
                  </tr>
                  <tr>
                    <td>键盘/鼠标时序特征</td>
                    <td>1 小时</td>
                    <td>实时分析后丢弃，不持久化存储</td>
                  </tr>
                </tbody>
              </table>
            </section>

            <section class="policy-section">
              <h2>🔐 数据安全措施</h2>
              <div class="security-grid">
                <div class="security-item">
                  <h4>传输安全</h4>
                  <p>✅ 全站 HTTPS 加密</p>
                  <p>✅ TLS 1.3 协议</p>
                </div>
                <div class="security-item">
                  <h4>存储安全</h4>
                  <p>✅ SQLite 数据库文件权限限制</p>
                  <p>✅ IP 地址部分脱敏存储</p>
                  <p>✅ 敏感字段单向哈希（SHA-256）</p>
                </div>
                <div class="security-item">
                  <h4>访问控制</h4>
                  <p>✅ 管理端需身份验证</p>
                  <p>✅ 操作日志审计</p>
                  <p>✅ 最小权限原则</p>
                </div>
                <div class="security-item">
                  <h4>数据隔离</h4>
                  <p>✅ 防护数据与用户内容分库</p>
                  <p>✅ 不向第三方服务传输</p>
                </div>
              </div>
            </section>

            <section class="policy-section">
              <h2>🚫 我们不会做的事</h2>
              <ul class="not-do-list">
                <li>❌ <strong>出售或共享数据</strong>：任何数据不会提供给第三方（除非法律要求）</li>
                <li>❌ <strong>广告追踪</strong>：不使用指纹数据进行广告投放或行为分析</li>
                <li>❌ <strong>身份关联</strong>：不尝试将指纹与真实身份关联</li>
                <li>❌ <strong>跨站追踪</strong>：数据仅在本站使用，不与其他网站共享</li>
                <li>❌ <strong>持久化标识</strong>：不在用户设备上存储永久追踪标识</li>
                <li>❌ <strong>记录具体内容</strong>：键盘鼠标数据仅收集时序模式，不记录您输入了什么、点击了什么</li>
              </ul>
            </section>

            <section class="policy-section">
              <h2>👤 您的权利</h2>
              <div class="rights-grid">
                <div class="right-item">
                  <h4>📧 查询权</h4>
                  <p>您可以请求查看我们存储的与您 IP 地址相关的数据</p>
                </div>
                <div class="right-item">
                  <h4>🗑️ 删除权</h4>
                  <p>您可以要求删除您的指纹和访问记录（需验证 IP 所有权）</p>
                </div>
                <div class="right-item">
                  <h4>🔓 解封申诉</h4>
                  <p>如认为封禁有误，可联系管理员申诉</p>
                </div>
                <div class="right-item">
                  <h4>🛡️ 拒绝权</h4>
                  <p>您可以通过停止访问本站来拒绝数据收集</p>
                </div>
              </div>
              <p class="contact-info">
                <strong>联系方式</strong>：请通过 GitHub Issues 或站点"关于"页面联系我们
              </p>
            </section>

            <section class="policy-section">
              <h2>🔧 技术细节（开发者）</h2>
              <details class="tech-details">
                <summary>点击展开技术实现说明</summary>
                <div class="tech-content">
                  <h4>指纹生成算法</h4>
                  <ul>
                    <li><strong>MinHash</strong>：用于字体列表、插件列表的相似度计算（128 哈希，Jaccard 相似度）</li>
                    <li><strong>PHash</strong>：用于 Canvas 图像的感知哈希（汉明距离 ≤5 视为相同）</li>
                    <li><strong>LSH (Locality Sensitive Hashing)</strong>：快速查找相似指纹（10 bands × 13 rows）</li>
                  </ul>
                  
                  <h4>封禁决策逻辑</h4>
                  <pre><code>违规积分 = Σ(事件积分 × 时间衰减)
封禁时长 = min(24h, 基础时长 × 1.5^(累计封禁次数))

事件积分示例：
- 访问 404：2 分
- 短时高频（>60 req/min）：10 分
- 自动化工具特征：15 分
- 登录暴力破解：30 分

封禁阈值：
- 第一次警告：30 分（封禁 30 分钟）
- 第二次：45 分（封禁 1 小时）
- 第三次及以后：60 分（封禁 2-24 小时）</code></pre>

                  <h4>数据库表结构</h4>
                  <ul>
                    <li><code>ip_events</code>：违规事件表（ip, kind, score, created_at）</li>
                    <li><code>ip_fingerprints</code>：指纹表（fp, ip, ua, components, minhash_sig, phash）</li>
                    <li><code>ip_bans</code>：封禁表（ip, reason, expires_at, strikes）</li>
                    <li><code>fp_lsh_buckets</code>：LSH 索引表（fp, band, bucket_hash）</li>
                  </ul>

                  <h4>开源代码</h4>
                  <p>本站完整源码已在 GitHub 开源：<a href="https://github.com/NoraStory/GithubHot" target="_blank">https://github.com/NoraStory/GithubHot</a></p>
                  <p>防护模块路径：<code>repo/internal/interfaces/httpapi/ipguard.go</code></p>
                </div>
              </details>
            </section>

            <section class="policy-section">
              <h2>📝 政策变更</h2>
              <p>
                本隐私政策可能不时更新。重大变更时，我们会在首页显著位置通知用户。
                继续使用本站即表示您接受更新后的政策。
              </p>
              <p class="version-info">
                当前版本：v1.0 | 生效日期：2025-01-15 | 
                <a href="https://github.com/NoraStory/GithubHot/commits/main/docs/privacy.md" target="_blank">查看历史版本</a>
              </p>
            </section>

            <section class="policy-section">
              <h2>⚖️ 适用法律</h2>
              <p>
                本隐私政策遵循《中华人民共和国网络安全法》《个人信息保护法》及相关法律法规。
                数据收集和处理基于"安全保障"的合法利益，且采用最小化原则。
              </p>
            </section>
          </div>
        </article>
      </div>
    </main>
  </div>
</template>

<style scoped>
.privacy-policy-content {
  max-width: 900px;
  margin: 0 auto;
  padding: 20px;
  line-height: 1.8;
  color: var(--anzhiyu-fontcolor);
}

.policy-section {
  margin-bottom: 48px;
}

.policy-section h2 {
  font-size: 1.8rem;
  font-weight: 700;
  margin-bottom: 20px;
  padding-bottom: 12px;
  border-bottom: 2px solid var(--anzhiyu-theme);
  color: var(--anzhiyu-fontcolor);
}

.policy-section h3 {
  font-size: 1.4rem;
  font-weight: 600;
  margin: 24px 0 16px;
  color: var(--anzhiyu-fontcolor);
}

.policy-section h4 {
  font-size: 1.1rem;
  font-weight: 600;
  margin-bottom: 12px;
  color: var(--anzhiyu-main);
}

.update-time {
  color: var(--anzhiyu-gray);
  font-size: 0.9rem;
  margin-top: 8px;
}

.info-card {
  background: var(--anzhiyu-card-bg);
  border: 1px solid var(--anzhiyu-card-border);
  border-radius: 8px;
  padding: 20px;
  margin-bottom: 20px;
}

.info-card.safe {
  background: var(--anzhiyu-green-op);
  border-color: var(--anzhiyu-green);
}

.info-card ul {
  margin: 12px 0;
  padding-left: 20px;
}

.info-card li {
  margin-bottom: 8px;
}

.note {
  margin-top: 16px;
  padding: 12px;
  background: var(--anzhiyu-yellow-op);
  border-left: 3px solid var(--anzhiyu-yellow);
  border-radius: 4px;
}

.usage-grid,
.security-grid,
.rights-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 20px;
  margin-top: 20px;
}

.usage-card,
.security-item,
.right-item {
  background: var(--anzhiyu-card-bg);
  border: 1px solid var(--anzhiyu-card-border);
  border-radius: 8px;
  padding: 20px;
}

.usage-card h4,
.security-item h4,
.right-item h4 {
  margin-bottom: 12px;
  color: var(--anzhiyu-main);
}

.usage-card ul,
.security-item ul {
  margin: 0;
  padding-left: 20px;
}

.usage-card li,
.security-item li {
  margin-bottom: 6px;
}

.security-item p {
  margin: 4px 0;
  font-size: 0.95rem;
}

.retention-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 20px;
  background: var(--anzhiyu-card-bg);
  border-radius: 8px;
  overflow: hidden;
}

.retention-table th,
.retention-table td {
  padding: 12px 16px;
  text-align: left;
  border-bottom: 1px solid var(--anzhiyu-card-border);
}

.retention-table th {
  background: var(--anzhiyu-theme-op);
  font-weight: 600;
  color: var(--anzhiyu-fontcolor);
}

.retention-table tbody tr:hover {
  background: var(--anzhiyu-theme-op-deep);
}

.not-do-list {
  list-style: none;
  padding: 0;
}

.not-do-list li {
  padding: 12px 16px;
  margin-bottom: 12px;
  background: var(--anzhiyu-card-bg);
  border: 1px solid var(--anzhiyu-card-border);
  border-radius: 6px;
}

.contact-info {
  margin-top: 20px;
  padding: 16px;
  background: var(--anzhiyu-blue-op);
  border-left: 3px solid var(--anzhiyu-blue);
  border-radius: 4px;
}

.tech-details {
  margin-top: 20px;
  background: var(--anzhiyu-card-bg);
  border: 1px solid var(--anzhiyu-card-border);
  border-radius: 8px;
  padding: 16px;
}

.tech-details summary {
  font-weight: 600;
  cursor: pointer;
  user-select: none;
  padding: 8px;
  border-radius: 4px;
}

.tech-details summary:hover {
  background: var(--anzhiyu-theme-op);
}

.tech-content {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid var(--anzhiyu-card-border);
}

.tech-content h4 {
  margin-top: 20px;
  margin-bottom: 12px;
}

.tech-content pre {
  background: var(--anzhiyu-code-bg);
  padding: 16px;
  border-radius: 6px;
  overflow-x: auto;
  margin: 12px 0;
}

.tech-content code {
  font-family: 'Courier New', monospace;
  font-size: 0.9rem;
  line-height: 1.6;
  color: var(--anzhiyu-fontcolor);
}

.version-info {
  margin-top: 16px;
  padding: 12px;
  background: var(--anzhiyu-theme-op);
  border-radius: 4px;
  font-size: 0.9rem;
}

.version-info a {
  color: var(--anzhiyu-main);
  text-decoration: underline;
}

@media (max-width: 768px) {
  .privacy-policy-content {
    padding: 12px;
  }

  .policy-section h2 {
    font-size: 1.5rem;
  }

  .usage-grid,
  .security-grid,
  .rights-grid {
    grid-template-columns: 1fr;
  }

  .retention-table {
    font-size: 0.85rem;
  }

  .retention-table th,
  .retention-table td {
    padding: 8px 10px;
  }
}
</style>
