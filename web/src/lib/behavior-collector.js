// 行为特征采集器 - 用于区分人类与自动化工具
// 仅收集时序和模式特征，不记录具体内容

class BehaviorCollector {
  constructor() {
    this.enabled = false
    this.keyboardTimings = [] // 按键间隔序列
    this.mousePoints = [] // 鼠标移动坐标
    this.clickPattern = [] // 点击模式
    this.scrollPattern = [] // 滚动行为
    
    this.lastKeyTime = 0
    this.lastMouseTime = 0
    
    // 采样限制（避免数据过大）
    this.maxKeyTimings = 50
    this.maxMousePoints = 100
    this.maxClicks = 30
    this.maxScrolls = 20
    
    // 采样率（降低性能影响）
    this.mouseSampleRate = 5 // 每5次移动采样1次
    this.mouseSampleCounter = 0
  }

  // 启动采集（仅在检测到异常行为时调用）
  start() {
    if (this.enabled) return
    this.enabled = true
    
    // 键盘事件监听
    window.addEventListener('keydown', this.handleKeyDown, true)
    
    // 鼠标移动监听（采样）
    window.addEventListener('mousemove', this.handleMouseMove, { passive: true, capture: true })
    
    // 点击监听
    window.addEventListener('mousedown', this.handleMouseDown, true)
    
    // 滚动监听
    window.addEventListener('wheel', this.handleWheel, { passive: true, capture: true })
    
    console.log('[BehaviorCollector] 已启动行为特征采集（不记录具体内容）')
  }

  // 停止采集
  stop() {
    if (!this.enabled) return
    this.enabled = false
    
    window.removeEventListener('keydown', this.handleKeyDown, true)
    window.removeEventListener('mousemove', this.handleMouseMove, true)
    window.removeEventListener('mousedown', this.handleMouseDown, true)
    window.removeEventListener('wheel', this.handleWheel, true)
    
    console.log('[BehaviorCollector] 已停止采集')
  }

  // 键盘事件处理 - 只记录按键间隔
  handleKeyDown = (e) => {
    if (!this.enabled) return
    
    const now = performance.now()
    if (this.lastKeyTime > 0) {
      const interval = now - this.lastKeyTime
      this.keyboardTimings.push(Math.round(interval))
      
      // 保留最新的N个样本
      if (this.keyboardTimings.length > this.maxKeyTimings) {
        this.keyboardTimings.shift()
      }
    }
    this.lastKeyTime = now
  }

  // 鼠标移动处理 - 采样坐标（相对窗口）
  handleMouseMove = (e) => {
    if (!this.enabled) return
    
    // 采样率控制
    this.mouseSampleCounter++
    if (this.mouseSampleCounter % this.mouseSampleRate !== 0) return
    
    const now = performance.now()
    const timeSinceLast = this.lastMouseTime > 0 ? now - this.lastMouseTime : 0
    
    // 标准化坐标（0-1000范围，避免泄漏真实屏幕尺寸）
    const normalizedX = Math.round((e.clientX / window.innerWidth) * 1000)
    const normalizedY = Math.round((e.clientY / window.innerHeight) * 1000)
    
    this.mousePoints.push({
      x: normalizedX,
      y: normalizedY,
      t: Math.round(timeSinceLast) // 与上次移动的时间差
    })
    
    if (this.mousePoints.length > this.maxMousePoints) {
      this.mousePoints.shift()
    }
    
    this.lastMouseTime = now
  }

  // 点击处理 - 记录位置和时间
  handleMouseDown = (e) => {
    if (!this.enabled) return
    
    const now = performance.now()
    const normalizedX = Math.round((e.clientX / window.innerWidth) * 1000)
    const normalizedY = Math.round((e.clientY / window.innerHeight) * 1000)
    
    this.clickPattern.push({
      x: normalizedX,
      y: normalizedY,
      btn: e.button, // 0=左键, 1=中键, 2=右键
      ts: Date.now()
    })
    
    if (this.clickPattern.length > this.maxClicks) {
      this.clickPattern.shift()
    }
  }

  // 滚动处理 - 记录方向和速度
  handleWheel = (e) => {
    if (!this.enabled) return
    
    this.scrollPattern.push({
      dx: Math.sign(e.deltaX), // -1/0/1
      dy: Math.sign(e.deltaY),
      ts: Date.now()
    })
    
    if (this.scrollPattern.length > this.maxScrolls) {
      this.scrollPattern.shift()
    }
  }

  // 获取当前采集的特征（用于上报）
  getFeatures() {
    return {
      keyboard: {
        intervals: this.keyboardTimings.slice(), // 按键间隔序列
        stats: this.computeKeyboardStats()
      },
      mouse: {
        points: this.mousePoints.slice(), // 移动轨迹
        stats: this.computeMouseStats()
      },
      clicks: this.clickPattern.slice(),
      scrolls: this.scrollPattern.slice()
    }
  }

  // 计算键盘统计特征
  computeKeyboardStats() {
    if (this.keyboardTimings.length < 5) return null
    
    const intervals = this.keyboardTimings
    const sum = intervals.reduce((a, b) => a + b, 0)
    const mean = sum / intervals.length
    
    // 标准差（衡量节奏稳定性）
    const variance = intervals.reduce((acc, val) => acc + Math.pow(val - mean, 2), 0) / intervals.length
    const stdDev = Math.sqrt(variance)
    
    // 最小/最大间隔
    const min = Math.min(...intervals)
    const max = Math.max(...intervals)
    
    return {
      count: intervals.length,
      mean: Math.round(mean),
      stdDev: Math.round(stdDev),
      min,
      max
    }
  }

  // 计算鼠标统计特征
  computeMouseStats() {
    if (this.mousePoints.length < 10) return null
    
    // 计算轨迹总长度（检测直线移动 vs 自然曲线）
    let totalDist = 0
    let totalTime = 0
    for (let i = 1; i < this.mousePoints.length; i++) {
      const prev = this.mousePoints[i - 1]
      const curr = this.mousePoints[i]
      const dx = curr.x - prev.x
      const dy = curr.y - prev.y
      totalDist += Math.sqrt(dx * dx + dy * dy)
      totalTime += curr.t
    }
    
    // 平均速度
    const avgSpeed = totalTime > 0 ? Math.round(totalDist / totalTime * 1000) : 0
    
    // 方向变化次数（人类轨迹会频繁微调方向）
    let dirChanges = 0
    for (let i = 2; i < this.mousePoints.length; i++) {
      const p1 = this.mousePoints[i - 2]
      const p2 = this.mousePoints[i - 1]
      const p3 = this.mousePoints[i]
      
      const v1x = p2.x - p1.x
      const v1y = p2.y - p1.y
      const v2x = p3.x - p2.x
      const v2y = p3.y - p2.y
      
      // 向量夹角变化
      const dot = v1x * v2x + v1y * v2y
      const len1 = Math.sqrt(v1x * v1x + v1y * v1y)
      const len2 = Math.sqrt(v2x * v2x + v2y * v2y)
      
      if (len1 > 0 && len2 > 0) {
        const cosAngle = dot / (len1 * len2)
        if (cosAngle < 0.9) { // 夹角 > 25度
          dirChanges++
        }
      }
    }
    
    return {
      points: this.mousePoints.length,
      totalDist: Math.round(totalDist),
      avgSpeed,
      dirChanges
    }
  }

  // 清空已采集的数据
  clear() {
    this.keyboardTimings = []
    this.mousePoints = []
    this.clickPattern = []
    this.scrollPattern = []
    this.lastKeyTime = 0
    this.lastMouseTime = 0
    this.mouseSampleCounter = 0
  }

  // 自动化检测分析（基于统计特征）
  detectAutomation() {
    const features = this.getFeatures()
    const flags = []
    
    // 键盘特征分析
    if (features.keyboard.stats) {
      const { mean, stdDev, min, max } = features.keyboard.stats
      
      // 异常1：按键间隔过于规律（stdDev < 10ms）
      if (stdDev < 10 && features.keyboard.stats.count > 10) {
        flags.push('keyboard-too-regular')
      }
      
      // 异常2：按键速度超人类极限（mean < 50ms）
      if (mean < 50 && features.keyboard.stats.count > 20) {
        flags.push('keyboard-too-fast')
      }
      
      // 异常3：完全固定间隔（min == max）
      if (min === max && features.keyboard.stats.count > 5) {
        flags.push('keyboard-fixed-interval')
      }
    }
    
    // 鼠标特征分析
    if (features.mouse.stats) {
      const { avgSpeed, dirChanges, points } = features.mouse.stats
      
      // 异常4：鼠标移动过于直线（方向变化少）
      if (points > 30 && dirChanges < points * 0.1) {
        flags.push('mouse-too-linear')
      }
      
      // 异常5：鼠标速度恒定（自动化工具特征）
      if (avgSpeed > 0 && this.mousePoints.length > 20) {
        const speeds = []
        for (let i = 1; i < this.mousePoints.length; i++) {
          const dx = this.mousePoints[i].x - this.mousePoints[i - 1].x
          const dy = this.mousePoints[i].y - this.mousePoints[i - 1].y
          const dist = Math.sqrt(dx * dx + dy * dy)
          if (this.mousePoints[i].t > 0) {
            speeds.push(dist / this.mousePoints[i].t * 1000)
          }
        }
        if (speeds.length > 10) {
          const speedMean = speeds.reduce((a, b) => a + b, 0) / speeds.length
          const speedStdDev = Math.sqrt(
            speeds.reduce((acc, s) => acc + Math.pow(s - speedMean, 2), 0) / speeds.length
          )
          if (speedStdDev < speedMean * 0.1) { // 变异系数 < 10%
            flags.push('mouse-constant-speed')
          }
        }
      }
    }
    
    // 点击特征分析
    if (features.clicks.length > 5) {
      const clickIntervals = []
      for (let i = 1; i < features.clicks.length; i++) {
        clickIntervals.push(features.clicks[i].ts - features.clicks[i - 1].ts)
      }
      
      // 异常6：点击间隔过于规律
      if (clickIntervals.length > 3) {
        const mean = clickIntervals.reduce((a, b) => a + b, 0) / clickIntervals.length
        const stdDev = Math.sqrt(
          clickIntervals.reduce((acc, val) => acc + Math.pow(val - mean, 2), 0) / clickIntervals.length
        )
        if (stdDev < 50 && mean < 1000) { // 小于50ms抖动且平均间隔<1秒
          flags.push('click-too-regular')
        }
      }
    }
    
    return {
      isAutomated: flags.length >= 2, // 2个以上异常标记为自动化
      flags,
      confidence: Math.min(flags.length / 3, 1) // 置信度 0-1
    }
  }
}

// 导出单例
export const behaviorCollector = new BehaviorCollector()
