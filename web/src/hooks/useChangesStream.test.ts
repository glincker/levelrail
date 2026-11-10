import { createElement, type ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  parseChangeTopics,
  parseHeartbeatMs,
  useChangesStream,
} from './useChangesStream'
import {
  CHANGE_TOPIC_KEYS,
  invalidateChangeTopic,
} from '../queries/changeTopics'
import { appKeys } from '../queries/apps'
import { nodeKeys } from '../queries/nodes'
import { domainKeys } from '../queries/domains'
import { alertRuleKeys } from '../queries/alerts'
import { pollMs, setChangeStreamHealthy, topicPollMs } from '../lib/pollTier'

type Listener = (m: MessageEvent<string>) => void

class FakeEventSource {
  static CLOSED = 2
  static instances: FakeEventSource[] = []
  readyState = 1
  closed = false
  onopen: (() => void) | null = null
  onmessage: Listener | null = null
  onerror: (() => void) | null = null
  listeners: Record<string, Listener> = {}
  url: string
  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }
  addEventListener(type: string, fn: Listener) {
    this.listeners[type] = fn
  }
  close() {
    this.closed = true
  }
}

describe('useChangesStream', () => {
  it('parses known topics and drops the rest', () => {
    expect(parseChangeTopics('{"topics":["apps","secrets","nodes"]}')).toEqual([
      'apps',
      'nodes',
    ])
    expect(parseChangeTopics('not json')).toEqual([])
    expect(parseChangeTopics('{"topics":"apps"}')).toEqual([])
  })

  it('maps topics to the real query key factories', () => {
    expect(CHANGE_TOPIC_KEYS.apps).toEqual([
      appKeys.list(),
      [...appKeys.all, 'pages'],
      [...appKeys.all, 'summary'],
    ])
    expect(CHANGE_TOPIC_KEYS.nodes).toEqual([nodeKeys.list()])
    expect(CHANGE_TOPIC_KEYS.domains).toEqual([domainKeys.all])
  })

  it('invalidates the mapped key and leaves others alone', () => {
    const qc = new QueryClient()
    qc.setQueryData(appKeys.list(), [])
    qc.setQueryData(nodeKeys.list(), [])
    qc.setQueryData(alertRuleKeys.list('web'), [])
    invalidateChangeTopic(qc, 'apps')
    expect(qc.getQueryState(appKeys.list())?.isInvalidated).toBe(true)
    expect(qc.getQueryState(nodeKeys.list())?.isInvalidated).toBe(false)
    invalidateChangeTopic(qc, 'alerts')
    expect(qc.getQueryState(alertRuleKeys.list('web'))?.isInvalidated).toBe(
      true,
    )
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    setChangeStreamHealthy(false)
  })

  it('stretches only topic polls while the stream is healthy', () => {
    const before = topicPollMs('steady')
    const plain = pollMs('steady')
    const live = topicPollMs('live')
    setChangeStreamHealthy(true)
    expect(topicPollMs('steady')).toBeGreaterThan(before)
    expect(topicPollMs('live')).toBe(live)
    expect(pollMs('steady')).toBe(plain)
    setChangeStreamHealthy(false)
    expect(topicPollMs('steady')).toBe(before)
  })

  it('reads the announced heartbeat', () => {
    expect(parseHeartbeatMs('{"heartbeat_ms":20000}')).toBe(20_000)
    expect(parseHeartbeatMs('{}')).toBeNull()
    expect(parseHeartbeatMs('x')).toBeNull()
  })

  it('reconnects when the stream goes silent past 2.5 heartbeats', () => {
    vi.useFakeTimers()
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    const qc = new QueryClient()
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: qc }, children)
    const { unmount } = renderHook(() => useChangesStream(true), { wrapper })
    const es = FakeEventSource.instances[0]
    if (!es) throw new Error('no stream opened')
    es.onopen?.()
    es.listeners.ping?.(
      new MessageEvent('ping', { data: '{"heartbeat_ms":1000}' }),
    )
    expect(topicPollMs('steady')).toBeGreaterThan(pollMs('steady'))
    vi.advanceTimersByTime(2400)
    expect(es.closed).toBe(false)
    vi.advanceTimersByTime(200)
    expect(es.closed).toBe(true)
    expect(topicPollMs('steady')).toBe(pollMs('steady'))
    vi.advanceTimersByTime(1000)
    expect(FakeEventSource.instances).toHaveLength(2)
    unmount()
  })
})
