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

  it('treats a known registry address as an image, and any registry after the docker word', () => {
    expect(parseMode('ghcr.io/org/app:1')).toEqual({
      kind: 'docker',
      term: 'ghcr.io/org/app:1',
    })
    expect(parseMode('docker registry.example.com/team/app')).toEqual({
      kind: 'docker',
      term: 'registry.example.com/team/app',
    })
    expect(parseMode('app.example.com/api')).toBeNull()
  })

  it('reads the tag prefix', () => {
    expect(parseMode('#prod')).toEqual({ kind: 'tags', term: 'prod' })
  })

  it('leaves ordinary searches alone', () => {
    expect(parseMode('firewall')).toBeNull()
    expect(parseMode('new app')).toBeNull()
    expect(parseMode('image sizes')).toBeNull()
    expect(parseMode('docker')).toBeNull()
    expect(parseMode('')).toBeNull()
  })
})
