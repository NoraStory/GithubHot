import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cleanEnvFlags, hashBytes } from './cleanEnv.js'

test('干净环境：三通道一致时不产生任何 flag', () => {
  const flags = cleanEnvFlags({
    canvasSupported: true,
    canvasMain: 'aaaa1111',
    canvasWorker: 'aaaa1111',
    iframeSupported: true,
    iframeDiff: [],
    audioSupported: true,
    audioDeterministic: true,
    audioReadConsistent: true
  })
  assert.deepEqual(flags, [])
})

test('Worker 与主线程画布不一致 → fpb_canvas_diverge', () => {
  const flags = cleanEnvFlags({
    canvasSupported: true,
    canvasMain: 'aaaa1111',
    canvasWorker: 'bbbb2222',
    iframeSupported: true,
    iframeDiff: [],
    audioSupported: true,
    audioDeterministic: true,
    audioReadConsistent: true
  })
  assert.deepEqual(flags, ['fpb_canvas_diverge'])
})

test('不支持 OffscreenCanvas → fpb_canvas_check_unsupported（不计分）', () => {
  const flags = cleanEnvFlags({
    canvasSupported: false,
    iframeSupported: true,
    iframeDiff: [],
    audioSupported: true,
    audioDeterministic: true,
    audioReadConsistent: true
  })
  assert.deepEqual(flags, ['fpb_canvas_check_unsupported'])
})

test('iframe 参照不一致 → fpb_iframe_diverge', () => {
  const flags = cleanEnvFlags({
    canvasSupported: true,
    canvasMain: 'x',
    canvasWorker: 'x',
    iframeSupported: true,
    iframeDiff: ['webdriver', 'plugins'],
    audioSupported: true,
    audioDeterministic: true,
    audioReadConsistent: true
  })
  assert.deepEqual(flags, ['fpb_iframe_diverge'])
})

test('音频随机化或读取层 patch → fpb_audio_diverge', () => {
  const base = {
    canvasSupported: true,
    canvasMain: 'x',
    canvasWorker: 'x',
    iframeSupported: true,
    iframeDiff: [],
    audioSupported: true
  }
  assert.deepEqual(cleanEnvFlags({ ...base, audioDeterministic: false, audioReadConsistent: true }),
    ['fpb_audio_diverge'])
  assert.deepEqual(cleanEnvFlags({ ...base, audioDeterministic: true, audioReadConsistent: false }),
    ['fpb_audio_diverge'])
})

test('多个通道同时命中时 flag 叠加（供服务端多证据互证）', () => {
  const flags = cleanEnvFlags({
    canvasSupported: true,
    canvasMain: 'a',
    canvasWorker: 'b',
    iframeSupported: true,
    iframeDiff: ['userAgent'],
    audioSupported: false
  })
  assert.deepEqual(flags, ['fpb_canvas_diverge', 'fpb_iframe_diverge', 'fpb_audio_check_unsupported'])
})

test('hashBytes：同输入同哈希、不同输入不同哈希（FNV-1a 稳定）', () => {
  const a = new Uint8Array([1, 2, 3, 4])
  assert.equal(hashBytes(a), hashBytes(new Uint8Array([1, 2, 3, 4])))
  assert.notEqual(hashBytes(a), hashBytes(new Uint8Array([1, 2, 3, 5])))
  assert.equal(hashBytes(new Uint8Array([0])).length, 8)
})
