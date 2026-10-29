import { describe, expect, it } from 'vitest'
import { parseMode } from './paletteMode'

describe('parseMode', () => {
  it('reads the docs and assistant prefixes', () => {
    expect(parseMode('/backup retention')).toEqual({
      kind: 'docs',
      term: 'backup retention',
    })
    expect(parseMode('@why is my deploy slow')).toEqual({
      kind: 'assistant',
      term: 'why is my deploy slow',
    })
  })

  it('reads the Docker Hub words and the plus prefix', () => {
    expect(parseMode('docker nginx')).toEqual({ kind: 'docker', term: 'nginx' })
    expect(parseMode('Deploy  redis')).toEqual({
      kind: 'docker',
      term: 'redis',
    })
    expect(parseMode('+postgres')).toEqual({ kind: 'docker', term: 'postgres' })
  })

  it('leaves ordinary searches alone', () => {
    expect(parseMode('firewall')).toBeNull()
    expect(parseMode('docker')).toBeNull()
    expect(parseMode('')).toBeNull()
  })
})
