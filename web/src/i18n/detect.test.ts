import { describe, expect, it } from 'vitest'
import { normalizeDetectedLanguage } from './detect'

describe('normalizeDetectedLanguage', () => {
  it.each([
    ['en', 'en'],
    ['en-US', 'en'],
    ['es', 'es'],
    ['es-MX', 'es'],
    ['pt', 'pt-BR'],
    ['pt-BR', 'pt-BR'],
    ['pt-br', 'pt-BR'],
    ['pt_BR', 'pt-BR'],
    ['pt-PT', 'pt-BR'],
    ['zh', 'zh-CN'],
    ['zh-CN', 'zh-CN'],
    ['zh-Hans', 'zh-CN'],
    ['zh-Hans-CN', 'zh-CN'],
    ['zh-SG', 'zh-CN'],
    ['zh-TW', 'zh-TW'],
    ['zh-Hant', 'zh-Hant'],
    ['ja-JP', 'ja'],
  ])('%s -> %s', (raw, expected) => {
    expect(normalizeDetectedLanguage(raw)).toBe(expected)
  })
})
