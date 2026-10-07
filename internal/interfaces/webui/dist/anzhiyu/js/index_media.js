/**
 * 首页背景媒体加载器（AnZhiYu index_media.js 改造版，本仓库重写为可读源码）
 *
 * 与原版的差异（性能向）：
 * 1. 视差效果用 requestAnimationFrame 合帧：mousemove/touchmove 只记录坐标，
 *    每帧最多写一次 transform，避免高频 getBoundingClientRect + 样式写入造成主线程抖动
 * 2. 页面隐藏（切标签/锁屏）暂停视频，恢复可见再播——后台标签不再持续下载+解码
 * 3. prefers-reduced-motion: reduce 时不自动播视频与视差，改用 poster/渐变静帧
 * 4. resize 自检从 setInterval(5s) 轮询改为仅在页面可见时低频自愈
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

  // ---------- 主流程：随机选片并注入媒体元素 ----------
  function pickOne(list) {
    return list[Math.floor(Math.random() * list.length)]
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
    if (src.indexOf('|') >= 0) src = pickOne(src.split('|'))

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
      var p = v.play()
      if (p && p.catch) p.catch(function () { v.muted = true; v.play().catch(function () {}) })
      v.addEventListener('loadeddata', hideLoader)
      v.addEventListener('canplay', hideLoader)
      attachParallax(container, v)
      initScrollFadeEffect()
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
