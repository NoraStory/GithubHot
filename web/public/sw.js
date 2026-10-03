// 背景视频本地缓存 Service Worker
// 策略：首次边下边播（不等整片），同时 SW 空闲时把整份片单预取进 Cache Storage，
// 之后（含刷新、随机到任何一首）直接本地出流，不再反复打远端。
// <video> 发起的是 no-cors 跨域请求，但 SW 内部用 cors 模式回源——
// Worker 已带 Access-Control-Allow-Origin: *，拿到的可读响应才能落缓存/切 Range。
const CACHE = 'gh-video-v3'
const PIC = 'https://pic.lololowe.com/video/x'
// 远程片单（本地 /video/ 由服务器直出，无需 SW 缓存；只缓存这 6 个远程源）
const PLAYLIST = [
  PIC + '/1.mp4', PIC + '/2.mp4', PIC + '/3.mp4', PIC + '/4.mp4', PIC + '/5.mp4', PIC + '/6.mp4',
]
let prefetching = false

self.addEventListener('install', (e) => { self.skipWaiting() })
self.addEventListener('activate', (e) => {
  e.waitUntil((async () => {
    const keys = await caches.keys()
    for (const k of keys) if (k !== CACHE) await caches.delete(k)
    await self.clients.claim()
  })())
})

self.addEventListener('fetch', (event) => {
  const req = event.request
  if (req.method !== 'GET') return
  const url = new URL(req.url)
  if (url.pathname.indexOf('.mp4') < 0) return
  if (url.host !== 'wanghaodatastorage.dpdns.org' && url.host !== 'pic.lololowe.com') return
  ensurePrefetch()
  event.respondWith(serveVideo(req))
})

// 空闲预取整份片单（省流模式跳过）。不阻塞应答；SW 被回收后下次请求会接力。
function ensurePrefetch() {
  if (prefetching) return
  if (self.navigator && self.navigator.connection && self.navigator.connection.saveData) return
  prefetching = true
  ;(async () => {
    const cache = await caches.open(CACHE)
    for (const url of PLAYLIST) {
      try {
        if (await cache.match(url)) continue
        const resp = await fetch(url, { mode: 'cors' })
        if (!resp.ok) continue
        const buf = await resp.arrayBuffer()
        await cache.put(url, new Response(buf, { headers: { 'Content-Type': 'video/mp4' } }))
      } catch (e) { /* 单个失败不阻塞其余 */ }
    }
  })().finally(() => { prefetching = false })
}

async function serveVideo(req) {
  const cache = await caches.open(CACHE)
  const url = req.url
  const rangeHeader = req.headers.get('Range')
  const cached = await cache.match(url, { ignoreVary: true })
  if (cached) {
    if (rangeHeader) return slice(cached, rangeHeader)
    return cached
  }
  try {
    // cors 回源：边下边播——拿到首包就立刻流式返回给 <video>，
    // 另一路在后台整片落缓存（下次访问直接本地出流）
    const resp = await fetch(url, { mode: 'cors' })
    if (!resp.ok || !resp.body) throw new Error('HTTP ' + resp.status)
    const headers = new Headers(resp.headers)
    headers.set('Accept-Ranges', 'bytes')
    const tee = resp.body.tee()
    tee[1].arrayBuffer().then(function (buf) {
      return cache.put(url, new Response(buf, { headers: { 'Content-Type': headers.get('Content-Type') || 'video/mp4' } }))
    }).catch(function () {})
    // 首访直接回 200 流（不等整片），播放器拿到首帧就 canplay
    return new Response(tee[0], { status: 200, headers: headers })
  } catch (e) {
    // 兜底：cors 回源失败时把原始请求原样转给网络——
    // 至少保证能播（不缓存），绝不把播放器卡死在 Response.error()
    try { return await fetch(req) } catch (e2) { return Response.error() }
  }
}

function parseRange(rangeHeader, total) {
  const m = /bytes=(\d*)-(\d*)/.exec(rangeHeader || '')
  if (!m) return null
  let start, end
  if (m[1] === '') {
    // bytes=-N：最后 N 字节
    start = Math.max(0, total - parseInt(m[2], 10))
    end = total - 1
  } else {
    start = parseInt(m[1], 10)
    end = m[2] ? Math.min(parseInt(m[2], 10), total - 1) : total - 1
  }
  if (start > end) return null
  return { start: start, end: end }
}

function headers206(buf, r, contentType) {
  return {
    'Content-Range': 'bytes ' + r.start + '-' + r.end + '/' + buf.byteLength,
    'Accept-Ranges': 'bytes',
    'Content-Length': r.end - r.start + 1,
    'Content-Type': contentType,
  }
}

function slice(cachedResp, rangeHeader) {
  return cachedResp.arrayBuffer().then(function (buf) {
    const r = parseRange(rangeHeader, buf.byteLength)
    if (!r) return cachedResp
    return new Response(buf.slice(r.start, r.end + 1), { status: 206, headers: headers206(buf, r, cachedResp.headers.get('Content-Type') || 'video/mp4') })
  })
}
