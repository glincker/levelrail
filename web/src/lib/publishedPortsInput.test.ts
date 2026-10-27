import { describe, expect, it } from 'vitest'
import {
  formatPublishedPortsInput,
  parsePublishedPorts,
} from './publishedPortsInput'

describe('parsePublishedPorts', () => {
  it('parses udp, mapped and default-tcp entries', () => {
    const r = parsePublishedPorts('53/udp, 5353:53/udp 8080')
    expect(r).toEqual({
      ok: true,
      ports: [
        { container_port: 53, host_port: 53, protocol: 'udp' },
        { container_port: 53, host_port: 5353, protocol: 'udp' },
        { container_port: 8080, host_port: 8080, protocol: 'tcp' },
      ],
    })
  })

  it('allows the same number on tcp and udp', () => {
    expect(parsePublishedPorts('53/tcp,53/udp').ok).toBe(true)
  })

  it('rejects duplicates, bad protocols and out of range ports', () => {
    expect(parsePublishedPorts('53/udp,53/udp')).toMatchObject({
      ok: false,
      error: { kind: 'duplicate' },
    })
    expect(parsePublishedPorts('53/sctp')).toMatchObject({
      ok: false,
      error: { kind: 'protocol' },
    })
    expect(parsePublishedPorts('0:53/udp')).toMatchObject({
      ok: false,
      error: { kind: 'range' },
    })
    expect(parsePublishedPorts('70000/udp')).toMatchObject({
      ok: false,
      error: { kind: 'range' },
    })
    expect(parsePublishedPorts('abc')).toMatchObject({
      ok: false,
      error: { kind: 'syntax' },
    })
  })

  it('round trips through the formatter', () => {
    const text = '53/udp, 5353:53/udp'
    const r = parsePublishedPorts(text)
    expect(r.ok && formatPublishedPortsInput(r.ports)).toBe(text)
  })
})
