import { describe, expect, it } from 'vitest'
import {
  describeTerminalExit,
  parseTerminalEvent,
  terminalSocketUrl,
} from './terminalSocket'

describe('terminalSocketUrl', () => {
  it('uses ws for a plain-http page', () => {
    expect(
      terminalSocketUrl(
        'web',
        { rows: 24, cols: 80 },
        { protocol: 'http:', host: 'localhost:8080' },
      ),
    ).toBe('ws://localhost:8080/api/v1/apps/web/terminal?rows=24&cols=80')
  })

  it('uses wss for an https page', () => {
    expect(
      terminalSocketUrl(
        'web',
        { rows: 40, cols: 120 },
        { protocol: 'https:', host: 'levelrail.example.com' },
      ),
    ).toBe(
      'wss://levelrail.example.com/api/v1/apps/web/terminal?rows=40&cols=120',
    )
  })

  it('escapes the app name', () => {
    expect(
      terminalSocketUrl(
        'web/prod',
        { rows: 24, cols: 80 },
        { protocol: 'http:', host: 'localhost' },
      ),
    ).toContain('/apps/web%2Fprod/terminal')
  })
})

describe('parseTerminalEvent', () => {
  it('reads an exit event', () => {
    expect(parseTerminalEvent('{"type":"exit","exit_code":7}')).toEqual({
      type: 'exit',
      exit_code: 7,
      message: undefined,
    })
  })

  it('returns null for anything else', () => {
    expect(parseTerminalEvent('not json')).toBeNull()
    expect(parseTerminalEvent('{"type":"resize"}')).toBeNull()
    expect(parseTerminalEvent('null')).toBeNull()
  })
})

describe('describeTerminalExit', () => {
  it('prefers a server-sent message', () => {
    expect(
      describeTerminalExit({ type: 'exit', message: 'node went away' }),
    ).toBe('node went away')
  })

  it('names a non-zero exit code', () => {
    expect(describeTerminalExit({ type: 'exit', exit_code: 3 })).toBe(
      'Session ended with exit code 3.',
    )
  })

  it('stays quiet about a clean exit', () => {
    expect(describeTerminalExit({ type: 'exit', exit_code: 0 })).toBe(
      'Session ended.',
    )
  })
})
