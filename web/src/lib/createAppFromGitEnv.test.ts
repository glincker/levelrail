import { describe, expect, it } from 'vitest'
import {
  envPayloadFrom,
  mergeEnvHintRows,
  missingEnvRows,
} from './createAppFromGit'

const hints = [
  { name: 'DATABASE_URL', required: true, looks_secret: true },
  { name: 'APP_NAME', required: true, looks_secret: false },
  { name: 'LOG_LEVEL', required: false, looks_secret: false },
]

describe('git env hint rows', () => {
  it('prefills only required names as empty rows and keeps typed values', () => {
    const rows = mergeEnvHintRows(
      [{ name: 'APP_NAME', value: 'demo', secret: false }],
      hints,
    )
    expect(rows.map((r) => r.name)).toEqual(['APP_NAME', 'DATABASE_URL'])
    expect(rows[0]?.value).toBe('demo')
  })

  it('reports empty rows as blocking and splits secrets from plain env', () => {
    const rows = mergeEnvHintRows([], hints)
    expect(missingEnvRows(rows)).toEqual(['DATABASE_URL', 'APP_NAME'])
    const filled = rows.map((r) => ({ ...r, value: 'x' }))
    expect(missingEnvRows(filled)).toEqual([])
    expect(envPayloadFrom(filled)).toEqual({
      env: { APP_NAME: 'x' },
      secrets: { DATABASE_URL: 'x' },
      secret_env: ['DATABASE_URL'],
    })
  })
})
