import type { PublishedPort } from '../types/publishedPort'

export type PublishedPortsParseError =
  | { kind: 'syntax'; entry: string }
  | { kind: 'protocol'; entry: string }
  | { kind: 'range'; entry: string }
  | { kind: 'duplicate'; entry: string }

export type PublishedPortsParse =
  | { ok: true; ports: PublishedPort[] }
  | { ok: false; error: PublishedPortsParseError }

const ENTRY = /^(?:(\d+):)?(\d+)(?:\/([a-z]+))?$/i
const MAX_PORT = 65535

function inRange(n: number): boolean {
  return Number.isInteger(n) && n >= 1 && n <= MAX_PORT
}

// Compose-style short syntax: "HOST:CONTAINER/proto", or "PORT/proto" when both
// sides match. Protocol defaults to tcp. Mirrors the server's own checks.
export function parsePublishedPorts(input: string): PublishedPortsParse {
  const ports: PublishedPort[] = []
  const seen = new Set<string>()
  for (const raw of input.split(/[\s,]+/)) {
    const entry = raw.trim()
    if (entry === '') continue
    const m = ENTRY.exec(entry)
    if (!m) return { ok: false, error: { kind: 'syntax', entry } }
    const proto = (m[3] ?? 'tcp').toLowerCase()
    if (proto !== 'tcp' && proto !== 'udp') {
      return { ok: false, error: { kind: 'protocol', entry } }
    }
    const container = Number(m[2])
    const host = m[1] === undefined ? container : Number(m[1])
    if (!inRange(host) || !inRange(container)) {
      return { ok: false, error: { kind: 'range', entry } }
    }
    const key = `${host}/${proto}`
    if (seen.has(key)) return { ok: false, error: { kind: 'duplicate', entry } }
    seen.add(key)
    ports.push({
      container_port: container,
      host_port: host,
      protocol: proto,
    })
  }
  return { ok: true, ports }
}

export function formatPublishedPortsInput(ports?: PublishedPort[]): string {
  return (ports ?? [])
    .map((p) =>
      p.host_port === p.container_port
        ? `${p.host_port}/${p.protocol}`
        : `${p.host_port}:${p.container_port}/${p.protocol}`,
    )
    .join(', ')
}
