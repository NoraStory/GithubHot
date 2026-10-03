// 背景视频本地缓存 Service Worker
// <video> 拉远端 mp4 每次都走网络会卡；这里对媒体域名做 runtime 缓存：
// 首次播放后整片进 Cache Storage，之后（含刷新、换片重播）直接本地出流。
// 注意：<video> 发起的是 no-cors 跨域请求，但 SW 内部用 cors 模式回源——
// Worker 已带 Access-Control-Allow-Origin: *，拿到的可读响应才能切片应答 Range。
const CACHE = 'gh-video-v1'
// 出错的条目（如上传中的坏文件）记住 URL，本次会话不再反复回源
const poisoned = new Set()

self.addEventListener('install', (e) => { self.skipWaiting() })
self.addEventListener('activate', (e) => { e.waitUntil(self.clients.claim()) })

self.addEventListener('fetch', (event) => {
  const req = event.request
  if (req.method !== 'GET') return
  const url = new URL(req.url)
  if (url.pathname.indexOf('.mp4') < 0) return
  if (url.host !== 'wanghaodatastorage.dpdns.org' && url.host !== 'pic.lololowe.com') return
  event.respondWith(serveVideo(req))
})

async function serveVideo(req) {
  const cache = await caches.open(CACHE)
  const url = req.url
  const rangeHeader = req.headers.get('Range')
  const cached = await cache.match(url, { ignoreVary: true })
  if (cached) {
    if (rangeHeader) return slice(cached, rangeHeader)
    return cached
  }
  if (poisoned.has(url)) return Response.error()
  try {
    // cors 回源：拿到可读 body 才能整片缓存/按 Range 切片
    const resp = await fetch(url, { mode: 'cors' })
    if (!resp.ok) throw new Error('HTTP ' + resp.status)
    // 先整片读入内存（单文件最大约 21MB，可接受）再同时落缓存和应答
    const buf = await resp.arrayBuffer()
    const headers = { 'Content-Type': 'video/mp4', 'Accept-Ranges': 'bytes' }
    cache.put(url, new Response(buf.slice(0), { headers: headers }))
    if (rangeHeader) return sliceBuf(buf, rangeHeader)
    return new Response(buf, { status: 200, headers: headers })
  } catch (e) {
    poisoned.add(url)
    return Response.error()
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

function sliceBuf(buf, rangeHeader) {
  const r = parseRange(rangeHeader, buf.byteLength)
  if (!r) return new Response(buf, { status: 200, headers: { 'Content-Type': 'video/mp4', 'Accept-Ranges': 'bytes' } })
  return new Response(buf.slice(r.start, r.end + 1), { status: 206, headers: headers206(buf, r, 'video/mp4') })
}
