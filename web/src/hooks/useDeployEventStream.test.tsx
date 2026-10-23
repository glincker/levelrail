import { act } from 'react'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useDeployEventStream } from './useDeployEventStream'

class StubEventSource {
  static instances: StubEventSource[] = []
  url: string
  closed = false
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((event: MessageEvent<string>) => void) | null = null

  constructor(url: string) {
    this.url = url
    StubEventSource.instances.push(this)
  }

  close(): void {
    this.closed = true
  }

  emit(data: unknown): void {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent<string>)
  }
}

function latest(): StubEventSource {
  const s = StubEventSource.instances[StubEventSource.instances.length - 1]
  if (!s) throw new Error('no stream opened')
  return s
}

function Probe({ id }: { id: string }) {
  const { state, connection } = useDeployEventStream(id)
  return (
    <div>
      <p>connection:{connection}</p>
      <p>verdict:{state.verdict}</p>
      <p>events:{state.events.map((e) => e.id).join('|')}</p>
    </div>
  )
}

const snapshot = {
  type: 'snapshot',
  timeline: {
    deploy_id: 'dep_1',
    app: 'web',
    verdict: 'running',
    live: true,
    frozen: false,
    started_at: '2026-10-01T12:00:00Z',
    trigger: { source: 'manual' },
    events: [
      {
        id: 'trigger',
        at: '2026-10-01T12:00:00Z',
        kind: 'trigger',
        phase: 'trigger',
        status: 'info',
        code: 'trigger.manual',
        title: 'x',
      },
    ],
    phases: [],
  },
}

describe('useDeployEventStream', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    StubEventSource.instances = []
  })

  it('follows the stream and closes it on the end message', () => {
    vi.stubGlobal('EventSource', StubEventSource)
    render(<Probe id="dep 1" />)
    expect(latest().url).toBe('/api/v1/deployments/dep%201/timeline/stream')
    act(() => {
      latest().onopen?.()
      latest().emit(snapshot)
      latest().emit({
        type: 'event',
        live: true,
        event: {
          id: 'cut',
          at: '2026-10-01T12:00:05Z',
          kind: 'cutover',
          phase: 'cutover',
          status: 'ok',
          code: 'cutover.passed',
          title: 'y',
        },
      })
    })
    expect(screen.getByText('connection:live')).toBeTruthy()
    expect(screen.getByText('events:trigger|cut')).toBeTruthy()
    expect(latest().closed).toBe(false)
    act(() => {
      latest().emit({ type: 'end', verdict: 'succeeded' })
    })
    expect(screen.getByText('connection:ended')).toBeTruthy()
    expect(screen.getByText('verdict:succeeded')).toBeTruthy()
    expect(latest().closed).toBe(true)
  })

  it('reports a connection error without losing events', () => {
    vi.stubGlobal('EventSource', StubEventSource)
    render(<Probe id="dep_1" />)
    act(() => {
      latest().emit(snapshot)
      latest().onerror?.()
    })
    expect(screen.getByText('connection:error')).toBeTruthy()
    expect(screen.getByText('events:trigger')).toBeTruthy()
  })

  it('resets when the deploy id changes', () => {
    vi.stubGlobal('EventSource', StubEventSource)
    const { rerender } = render(<Probe id="dep_1" />)
    act(() => {
      latest().emit(snapshot)
    })
    rerender(<Probe id="dep_2" />)
    expect(screen.getByText('events:')).toBeTruthy()
    expect(latest().url).toContain('dep_2')
  })
})
