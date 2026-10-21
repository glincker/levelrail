import { describe, expect, it } from 'vitest'
import { buildDeployReport, redactSecrets, REDACTED } from './deployReport'

describe('redactSecrets', () => {
  it('masks sensitive key value pairs', () => {
    expect(redactSecrets('DB_PASSWORD=hunter2 PORT=3000')).toBe(
      `DB_PASSWORD=${REDACTED} PORT=3000`,
    )
    expect(redactSecrets('"api_key": "abc123"')).toContain(REDACTED)
    expect(redactSecrets('"api_key": "abc123"')).not.toContain('abc123')
  })

  it('masks bearer tokens, url credentials and token shapes', () => {
    expect(redactSecrets('Authorization: Bearer abcdefgh12345')).not.toContain(
      'abcdefgh12345',
    )
    expect(redactSecrets('postgres://user:s3cretpw@db:5432/x')).toBe(
      `postgres://user:${REDACTED}@db:5432/x`,
    )
    expect(redactSecrets('pushed ghp_abcdefghijklmnopqrstuvwx ok')).toBe(
      `pushed ${REDACTED} ok`,
    )
  })

  it('masks literal env values anywhere', () => {
    expect(
      redactSecrets('connect failed to zebra-quartz-77', ['zebra-quartz-77']),
    ).toBe(`connect failed to ${REDACTED}`)
  })

  it('keeps short literal values to avoid shredding the log', () => {
    expect(redactSecrets('port 3000 open', ['3000'])).toBe('port 3000 open')
  })
})

describe('buildDeployReport', () => {
  it('limits to the last 100 lines and redacts them', () => {
    const lines = Array.from({ length: 150 }, (_, i) => `line ${i}`)
    lines.push('TOKEN=abc')
    const md = buildDeployReport({
      appName: 'web',
      deployId: 'd1',
      stageLabel: 'Build',
      error: 'boom',
      causeTitle: 'Port mismatch',
      lines,
      config: { Image: 'img:1', Empty: undefined },
    })
    expect(md).toContain('# Deploy report: web')
    expect(md).toContain('- Failing stage: Build')
    expect(md).toContain('## Last 100 log lines')
    expect(md).not.toContain('line 10\n')
    expect(md).toContain('line 149')
    expect(md).toContain(`TOKEN=${REDACTED}`)
    expect(md).toContain('- Image: img:1')
    expect(md).not.toContain('Empty')
  })
})
