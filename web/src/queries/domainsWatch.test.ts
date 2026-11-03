import { describe, expect, it, vi } from 'vitest'
import {
  applyWatchMessage,
  createWatchStore,
  parseWatchMessage,
  type WatchEntry,
} from './domainsWatch'

const entry = (over: Partial<WatchEntry> = {}): WatchEntry => ({
  domain: 'App.Example.com',
  state: 'waiting_dns',
  ...over,
})

describe('parseWatchMessage', () => {
  it('accepts a snapshot and rejects malformed data', () => {
    const ok = parseWatchMessage(
      JSON.stringify({ kind: 'snapshot', entry: entry() }),
    )
    expect(ok?.kind).toBe('snapshot')
    expect(parseWatchMessage('not json')).toBeNull()
    expect(parseWatchMessage('{"kind":"x","entry":{}}')).toBeNull()
    expect(
      parseWatchMessage(
        JSON.stringify({
          kind: 'update',
          entry: entry({ state: 'bogus' as never }),
        }),
      ),
    ).toBeNull()
  })
})

describe('applyWatchMessage', () => {
  it('adds on snapshot, replaces on update, drops on removed', () => {
    let m = applyWatchMessage(new Map(), { kind: 'snapshot', entry: entry() })
    expect(m.get('app.example.com')?.state).toBe('waiting_dns')
    m = applyWatchMessage(m, {
      kind: 'update',
      entry: entry({ state: 'propagating' }),
    })
    expect(m.get('app.example.com')?.state).toBe('propagating')
    m = applyWatchMessage(m, { kind: 'removed', entry: entry() })
    expect(m.size).toBe(0)
  })
})

class FakeSource {
  static CLOSED = 2
  readyState = 0
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  handlers = new Map<string, (ev: MessageEvent<string>) => void>()
  closed = false
  addEventListener(name: string, h: (ev: MessageEvent<string>) => void) {
    this.handlers.set(name, h)
  }
  close() {
    this.closed = true
    this.readyState = 2
  }
  emit(kind: string, e: WatchEntry) {
    this.handlers.get('watch')?.({
      data: JSON.stringify({ kind, entry: e }),
    } as MessageEvent<string>)
  }
}

function first(sources: FakeSource[]): FakeSource {
  const s = sources[0]
  if (!s) throw new Error('no source opened')
  return s
}

function setup() {
  vi.stubGlobal('EventSource', FakeSource)
  const sources: FakeSource[] = []
  const timers: (() => void)[] = []
  const store = createWatchStore(
    () => {
      const s = new FakeSource()
      sources.push(s)
      return s as unknown as EventSource
    },
    ((fn: () => void) => {
      timers.push(fn)
      return 1
    }) as unknown as typeof setTimeout,
    () => undefined,
  )
  return { store, sources, timers }
}

describe('createWatchStore', () => {
  it('shares one stream, applies events and closes with the last subscriber', () => {
    const { store, sources } = setup()
    const a = store.subscribe(() => undefined)
    const b = store.subscribe(() => undefined)
    expect(sources).toHaveLength(1)
    first(sources).onopen?.()
    first(sources).emit('snapshot', entry())
    first(sources).emit('update', entry({ state: 'waiting_certificate' }))
    expect(store.getSnapshot().status).toBe('live')
    expect(store.getSnapshot().entries.get('app.example.com')?.state).toBe(
      'waiting_certificate',
    )
    first(sources).emit('removed', entry())
    expect(store.getSnapshot().entries.size).toBe(0)
    a()
    expect(first(sources).closed).toBe(false)
    b()
    expect(first(sources).closed).toBe(true)
  })

  it('goes quiet when the endpoint answers an error before opening', () => {
    const { store, sources, timers } = setup()
    store.subscribe(() => undefined)
    first(sources).readyState = 2
    first(sources).onerror?.()
    expect(store.getSnapshot().status).toBe('unavailable')
    expect(timers).toHaveLength(0)
  })

  it('reconnects with backoff after an established stream drops', () => {
    const { store, sources, timers } = setup()
    store.subscribe(() => undefined)
    first(sources).onopen?.()
    first(sources).readyState = 2
    first(sources).onerror?.()
    expect(timers).toHaveLength(1)
    timers[0]?.()
    expect(sources).toHaveLength(2)
  })
})
