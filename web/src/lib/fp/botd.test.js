import { test } from 'node:test'
import assert from 'node:assert/strict'
import { botdFlags, normalizeName } from './botd.js'

test('normalizeName：驼峰与符号统一成下划线段', () => {
  assert.equal(normalizeName('webDriver'), 'web_driver')
  assert.equal(normalizeName('distinctiveProperties'), 'distinctive_properties')
  assert.equal(normalizeName('headless'), 'headless')
  assert.equal(normalizeName('-CEF-'), 'cef')
  assert.equal(normalizeName(''), '')
})

test('BotD 命中：总体 botKind 与逐项检测器都展开为 flag', () => {
  const flags = botdFlags(
    { bot: true, botKind: 'headless' },
    { webDriver: { bot: true, botKind: 'selenium' }, headless: { bot: true, botKind: 'headless' }, rtt: { bot: false } }
  )
  assert.deepEqual(flags, ['botd_headless', 'botd_web_driver_1', 'botd_headless_1'])
})

test('BotD 未命中 → 零 flag（真人浏览器红线）', () => {
  assert.deepEqual(botdFlags({ bot: false }, { webDriver: { bot: false } }), [])
  assert.deepEqual(botdFlags(undefined, undefined), [])
})

test('BotD 命中但无 kind → botd_detected 兜底', () => {
  assert.deepEqual(botdFlags({ bot: true }), ['botd_detected'])
})

test('flag 去重（同一被多个检测器命中）', () => {
  const flags = botdFlags({ bot: true, botKind: 'selenium' }, {
    webDriver: { bot: true, botKind: 'selenium' },
    distinctiveProperties: { bot: true, botKind: 'selenium' }
  })
  assert.equal(new Set(flags).size, flags.length)
  assert.ok(flags.includes('botd_selenium'))
})
