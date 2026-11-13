import { readdirSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { LOADER_LANGUAGES } from './lazyBackend'
import { SUPPORTED_LANGUAGES } from './supported'

const folders = readdirSync(join(__dirname, '..', 'locales'))
const files = readdirSync(join(__dirname, 'loaders'))
  .filter((f) => f.endsWith('.ts'))
  .map((f) => f.replace(/\.ts$/, ''))

describe('language loaders', () => {
  it('has a loader for every locale folder and every supported language', () => {
    expect([...LOADER_LANGUAGES].sort()).toEqual([...folders].sort())
    expect([...files].sort()).toEqual([...folders].sort())
    expect([...SUPPORTED_LANGUAGES].sort()).toEqual([...folders].sort())
  })
})
