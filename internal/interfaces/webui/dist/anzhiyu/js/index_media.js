/**
 * 首页背景媒体加载器（AnZhiYu index_media.js 改造版，本仓库重写为可读源码）
 *
 * 与原版的差异（性能向）：
 * 1. 预热池随机选片：≤4 支已进浏览器缓存的池子内随机（排除上一支），后台把池外
 *    随机视频拉进缓存轮换补充——每次刷新随机换片且零缓冲；主视频完整缓冲后才
 *    串行预热 1 支（并发预取抢公网上行是播放后变卡根因），不绕过服务端中间件
 * 2. 视差效果用 requestAnimationFrame 合帧：mousemove/touchmove 只记录坐标，
 *    每帧最多写一次 transform，避免高频 getBoundingClientRect + 样式写入造成主线程抖动
 * 3. 页面隐藏（切标签/锁屏）暂停视频，恢复可见再播——后台标签不再持续下载+解码
 * 4. prefers-reduced-motion: reduce 时不自动播视频与视差，改用 poster/渐变静帧
 * 5. resize 自检从 setInterval(5s) 轮询改为仅在页面可见时低频自愈
 * 媒体选片契约不变：容器 data-landscape-video / data-portrait-video（"|" 分隔随机选一）。
 */
(function () {
  'use strict'

  var lastOrientation = null
  var scrollFadeAttached = false
  var rafPending = false
  var pointer = { x: 0.5, y: 0.5 }
  var currentVideo = null

  function reducedMotion() {
    return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  }

  function throttle(func, limit) {
    var lastFunc, lastRan
    return function () {
      var context = this, args = arguments
      if (!lastRan) { func.apply(context, args); lastRan = Date.now() }
      else {
        clearTimeout(lastFunc)
        lastFunc = setTimeout(function () {
          if (Date.now() - lastRan >= limit) { func.apply(context, args); lastRan = Date.now() }
        }, limit - (Date.now() - lastRan))
      }
    }
  }

  // ---------- 滚动淡出（原版行为保留，passive 监听） ----------
  function handleScrollFade() {
    var el = document.querySelector('#home-media-container .home-media')
    if (!el) return
    var opacity = 1 - window.scrollY / window.innerHeight
    el.style.opacity = Math.max(0, Math.min(1, opacity))
  }
  function initScrollFadeEffect() {
    var container = document.getElementById('home-media-container')
    if (!container) return
    if (!scrollFadeAttached) {
      window.addEventListener('scroll', throttle(handleScrollFade, 50), { passive: true })
      scrollFadeAttached = true
    }
    handleScrollFade()
  }

  // ---------- 视差（rAF 合帧） ----------
  function scheduleParallax(el, isPortrait) {
    if (rafPending) return
    rafPending = true
    requestAnimationFrame(function () {
      rafPending = false
      var base = isPortrait ? 1.05 : 1
      var dx = (pointer.x - 0.5) * 5   // ±2.5% 位移
      var dy = (pointer.y - 0.5) * 5
      el.style.transform = 'translate(' + dx + '%, ' + dy + '%) scale(' + (base + 0.05) + ')'
    })
  }
  function attachParallax(container, el) {
    if (reducedMotion()) return
    var isPortrait = window.innerHeight > window.innerWidth
    var isMobile = /Mobi|Android/i.test(navigator.userAgent)
    if (isMobile) {
      // 移动端陀螺仪（iOS 需授权）；失败退化为无视差，不再用 touchmove 高频写样式
      function onOrient(e) {
        pointer.x = 0.5 + ((e.gamma || 0) / 90) * 0.5
        pointer.y = 0.5 + ((e.beta || 0) / 180) * 0.5
        scheduleParallax(el, isPortrait)
      }
      var attach = function () { window.addEventListener('deviceorientation', onOrient) }
      if (typeof DeviceOrientationEvent !== 'undefined' && typeof DeviceOrientationEvent.requestPermission === 'function') {
        DeviceOrientationEvent.requestPermission().then(function (s) { if (s === 'granted') attach() }).catch(function () {})
      } else if ('DeviceOrientationEvent' in window) {
        attach()
      }
      return
    }
    container.addEventListener('mousemove', function (e) {
      var rect = container.getBoundingClientRect()
      pointer.x = (e.clientX - rect.left) / rect.width
      pointer.y = (e.clientY - rect.top) / rect.height
      scheduleParallax(el, isPortrait)
    }, { passive: true })
    container.addEventListener('mouseleave', function () { el.style.transform = 'scale(1)' })
  }

  // ---------- 页面可见性：隐藏即暂停（省带宽+CPU），恢复续播 ----------
  function attachVisibility(el) {
    document.addEventListener('visibilitychange', function () {
      if (!el.isConnected) return
      if (document.visibilityState === 'hidden') {
        el.pause()
      } else if (el.paused) {
        var p = el.play()
        if (p && p.catch) p.catch(function () {})
      }
    })
  }

  // ---------- 主流程：随机选片 + 预热池 ----------
  // 需求：每次刷新随机换片，但不能重新缓冲卡顿。方案 = 预热池随机：
  //   - localStorage 维护 ≤4 支"已进浏览器缓存"的视频（条目 24h TTL）
  //   - 刷新时从池内随机抽取（排除上一次播放的那支）→ 命中 HTTP 缓存秒开
  //   - 播放稳定后，后台把一支"池外随机"视频拉进缓存并入池、淘汰最旧
  //     → 池子持续轮换，每次刷新看起来都是随机的新背景，且全程零缓冲
  //   - 冷启动（池空）才走一次网络加载，属不可避免的一次性成本
  var WARM_KEY = 'gh_bg_warm_'
  var WARM_MAX = 4
  var WARM_TTL = 86400000 // 24h

  function warmList(orientation) {
    try {
      var arr = JSON.parse(localStorage.getItem(WARM_KEY + orientation) || '[]')
      var now = Date.now()
      return arr.filter(function (e) {
        return e && e.src && now - e.ts < WARM_TTL
      }).map(function (e) { return e.src })
    } catch (e) { return [] }
  }
  function warmSave(orientation, srcs) {
    try {
      localStorage.setItem(WARM_KEY + orientation, JSON.stringify(
        srcs.map(function (s) { return { src: s, ts: Date.now() } })
      ))
    } catch (e) { /* 存储不可用：退化为每次冷加载 */ }
  }
  function warmAdd(orientation, src) {
    var w = warmList(orientation)
    if (w.indexOf(src) >= 0) return
    w.push(src)
    while (w.length > WARM_MAX) w.shift()
    warmSave(orientation, w)
  }
  function lastPlayed(orientation) {
    try { return localStorage.getItem(WARM_KEY + 'last_' + orientation) || '' } catch (e) { return '' }
  }
  function pickRandom(list, orientation) {
    var warm = warmList(orientation).filter(function (s) { return list.indexOf(s) >= 0 })
    // 排除上一次播放的那支（池内还有其他候选时），避免"刷新还是同一支"
    if (warm.length > 1) warm = warm.filter(function (s) { return s !== lastPlayed(orientation) })
    var pool = warm.length ? warm : list
    return pool[Math.floor(Math.random() * pool.length)]
  }
  // 后台预热（带宽优先版）：当前视频**完整缓冲后**才允许串行预热一支池外视频，
  // 且每次页面加载至多 1 支——并发预取流会与播放抢服务器公网上行（轻量服务器
  // 仅几 Mbps），这正是"播放一段时间后变卡"的主因。移动端/省流模式直接跳过。
  function startPrefetch(list, orientation) {
    if (reducedMotion()) return
    var budget = 1
    var busy = false
    var timer = setInterval(function () {
      if (budget <= 0 || busy || document.visibilityState === 'hidden') return
      if (!currentVideo || !currentVideo.isConnected || currentVideo.readyState < 4) return
      // 关键门控：主视频 buffered 覆盖全片（循环回放不再依赖网络）才允许预热
      try {
        var b = currentVideo.buffered
        if (!(b.length > 0 && b.end(b.length - 1) >= (currentVideo.duration || 0) - 0.5)) return
      } catch (e) { return }
      try {
        var conn = navigator.connection || {}
        if (conn.saveData || /^2g/i.test(conn.effectiveType || '')) return
      } catch (e) { }
      var warm = warmList(orientation)
      var candidates = list.filter(function (s) { return warm.indexOf(s) < 0 })
      if (!candidates.length) { clearInterval(timer); return }
      budget--
      busy = true
      var src = candidates[Math.floor(Math.random() * candidates.length)]
      var pv = document.createElement('video')
      pv.preload = 'auto'
      pv.muted = true
      var done = false
      var finish = function (ok) {
        if (done) return
        done = true
        if (ok) warmAdd(orientation, src)
        pv.removeAttribute('src')
        pv.load()
        busy = false
      }
      pv.addEventListener('canplaythrough', function () { finish(true) }, { once: true })
      pv.addEventListener('error', function () { finish(false) }, { once: true })
      setTimeout(function () { finish(false) }, 20000) // 慢源预热超时放弃，不再占带宽
      pv.src = src
    }, 6000)
  }

  function initResponsiveBackground() {
    var container = document.getElementById('home-media-container')
    if (!container) return
    var isPortrait = window.innerHeight > window.innerWidth
    var orientation = isPortrait ? 'portrait' : 'landscape'
    if (lastOrientation === orientation) return
    lastOrientation = orientation

    var existing = container.querySelector('.home-media')
    var loader = container.querySelector('.custom-loader')
    if (existing) existing.remove()
    if (loader) loader.remove()

    var src = isPortrait
      ? (container.dataset.portraitVideo || container.dataset.portraitImg)
      : (container.dataset.landscapeVideo || container.dataset.landscapeImg)
    var poster = isPortrait ? container.dataset.portraitPoster : container.dataset.landscapePoster
    if (!src) return
    var fullList = src.indexOf('|') >= 0 ? src.split('|') : [src]
    src = pickRandom(fullList, orientation)

    var loaderBox = document.createElement('div')
    loaderBox.className = 'custom-loader'
    var loaderAnim = document.createElement('div')
    loaderAnim.className = 'loader-animation'
    if (poster) loaderAnim.style.backgroundImage = 'url(' + poster + ')'
    loaderBox.appendChild(loaderAnim)
    container.appendChild(loaderBox)

    function hideLoader() {
      loaderBox.style.opacity = '0'
      setTimeout(function () {
        if (loaderBox.parentNode) loaderBox.parentNode.removeChild(loaderBox)
        var siteInfo = document.getElementById('site-info')
        if (siteInfo) { siteInfo.style.opacity = '1'; siteInfo.style.pointerEvents = 'auto' }
      }, 500)
    }
    setTimeout(hideLoader, 2000) // 慢源兜底：最迟 2s 揭开正文

    var isVideo = !isPortrait
      ? !!container.dataset.landscapeVideo
      : !!container.dataset.portraitVideo

    if (isVideo && reducedMotion()) {
      // 减弱动态：不播视频，仅留加载层渐隐（背景由 CSS 渐变兜底）
      isVideo = false
      src = poster || ''
      if (!src) { hideLoader(); return }
    }

    if (isVideo) {
      var v = document.createElement('video')
      v.className = 'home-media'
      v.style.cssText = 'width:100%;height:100%;object-fit:cover'
      v.autoplay = true
      v.muted = true
      v.loop = true
      v.playsInline = true
      v.setAttribute('playsinline', '')
      v.setAttribute('webkit-playsinline', '')
      v.setAttribute('muted', '')
      v.preload = 'auto'
      var source = document.createElement('source')
      source.src = src
      source.type = src.toLowerCase().indexOf('.webm') >= 0 ? 'video/webm' : 'video/mp4'
      v.appendChild(source)
      // 初始缩放入场（原版行为）：加载完成后 1.2 → 1 缓动收拢
      if (!reducedMotion()) {
        v.style.transform = 'scale(1.2)'
        v.style.transition = 'transform 0.5s ease-out'
        v.addEventListener('loadeddata', function () { v.style.transform = 'scale(1)' })
      }
      container.appendChild(v)
      currentVideo = v
      attachVisibility(v)
      // WeChat 内自动播放策略兼容（原版行为保留）
      if (typeof WeixinJSBridge === 'object' && typeof WeixinJSBridge.invoke === 'function') {
        WeixinJSBridge.invoke('getNetworkType', {}, function () { v.play() })
      }
      try { localStorage.setItem(WARM_KEY + 'last_' + orientation, src) } catch (e) { }
      var p = v.play()
      if (p && p.catch) p.catch(function () { v.muted = true; v.play().catch(function () {}) })
      v.addEventListener('loadeddata', hideLoader)
      v.addEventListener('canplay', hideLoader)
      attachParallax(container, v)
      initScrollFadeEffect()
      startPrefetch(fullList, orientation)
    } else {
      var img = document.createElement('img')
      img.className = 'home-media'
      img.style.cssText = 'width:100%;height:100%;object-fit:cover'
      img.loading = 'eager'
      img.src = src
      container.appendChild(img)
      img.addEventListener('load', hideLoader)
      img.addEventListener('error', hideLoader)
      initScrollFadeEffect()
    }
  }

  function initMedia() {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', function () {
        initResponsiveBackground()
        initScrollFadeEffect()
      })
    } else {
      initResponsiveBackground()
      initScrollFadeEffect()
    }
  }
  initMedia()

  // ---------- 自愈：resize / bfcache 恢复 / 路由返回 ----------
  var resizeTimer
  window.addEventListener('resize', function () {
    clearTimeout(resizeTimer)
    resizeTimer = setTimeout(function () {
      var orientation = window.innerHeight > window.innerWidth ? 'portrait' : 'landscape'
      if (lastOrientation !== orientation) initResponsiveBackground()
      else initScrollFadeEffect()
    }, 500)
  })
  window.addEventListener('pageshow', function (event) {
    if (event.persisted && location.pathname === '/') {
      lastOrientation = null
      initResponsiveBackground()
      setTimeout(initScrollFadeEffect, 300)
    }
  })
  window.addEventListener('popstate', function () {
    if (location.pathname === '/') {
      setTimeout(function () {
        var c = document.getElementById('home-media-container')
        if (c && !c.querySelector('.home-media')) {
          lastOrientation = null
          initResponsiveBackground()
        }
        initScrollFadeEffect()
      }, 300)
    }
  })
})()
