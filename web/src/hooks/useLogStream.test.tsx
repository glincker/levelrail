import { act } from 'react'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, it, vi } from 'vitest'
import { useLogStream } from './useLogStream'

// jsdom has no native EventSource; this stub gives a test direct control
// over onopen/onmessage the same way real backends drive it: an initial
// backfill burst, then live lines, and (via a second emitOpen on the
// same instance) a reconnect exactly as the browser's own EventSource
// reuses one object across a network drop rather than constructing a
// new one.
class StubEventSource {
  static instances: StubEventSource[] = []
  url: string
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((event: MessageEvent<string>) => void) | null = null

  constructor(url: string) {
    this.url = url
    StubEventSource.instances.push(this)
  }

  close(): void {
    // no-op: nothing to assert on close for these tests
  }

  emitOpen(): void {
    this.onopen?.()
  }

  emitMessage(data: string): void {
    this.onmessage?.({ data } as MessageEvent<string>)
  }
}

function logEvent(line: string, stream: 'stdout' | 'stderr' = 'stdout'): string {
  return JSON.stringify({ line, stream })
}

function latestSource(): StubEventSource {
  const source = StubEventSource.instances[StubEventSource.instances.length - 1]
  if (!source) throw new Error('no StubEventSource was constructed')
  return source
}

function LogProbe({ url }: { url: string }) {
  const { lines, connectionState } = useLogStream(url)
  return (
    <div>
      <p>state:{connectionState}</p>
      <p>lines:{lines.map((l) => l.line).join('|')}</p>
    </div>
  )
}

describe('useLogStream', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    StubEventSource.instances = []
  })

  it('does not suppress anything on the very first connection', () => {
    vi.stubGlobal('EventSource', StubEventSource)
    render(<LogProbe url="http://test/logs" />)
    const source = latestSource()

    act(() => {
      source.emitOpen()
      source.emitMessage(logEvent('line 1'))
    })

    screen.getByText('state:open')
    screen.getByText('lines:line 1')
  })

  it('drops a reconnect backfill burst that duplicates already-rendered lines', () => {
    vi.stubGlobal('EventSource', StubEventSource)
    render(<LogProbe url="http://test/logs" />)
    const source = latestSource()

    act(() => {
      source.emitOpen()
      source.emitMessage(logEvent('line 1'))
      source.emitMessage(logEvent('line 2'))
    })
    screen.getByText('lines:line 1|line 2')

    // A network blip: EventSource reuses the same instance and fires
    // onopen again once reconnected. Neither real backend supports
    // Last-Event-ID resume (see useLogStream.ts's own doc comment), so
    // the server resends its whole backfill/replay burst verbatim
    // before anything genuinely new.
    act(() => {
      source.emitOpen()
      source.emitMessage(logEvent('line 1'))
      source.emitMessage(logEvent('line 2'))
      source.emitMessage(logEvent('line 3'))
    })

    screen.getByText('lines:line 1|line 2|line 3')
  })

  it('falls back to appending verbatim once the resumed burst diverges from what is buffered', () => {
    vi.stubGlobal('EventSource', StubEventSource)
    render(<LogProbe url="http://test/logs" />)
    const source = latestSource()

    act(() => {
      source.emitOpen()
      source.emitMessage(logEvent('line 1'))
    })

    // The gap was long enough that nothing in the resumed burst matches
    // what's already buffered (e.g. the backfill window rolled past it
    // entirely): the mismatch on the very first compared line must not
    // swallow it.
    act(() => {
      source.emitOpen()
      source.emitMessage(logEvent('a completely different line'))
    })

    screen.getByText('lines:line 1|a completely different line')
  })

  it('stops comparing, and appends everything, once the overlap is exhausted', () => {
    vi.stubGlobal('EventSource', StubEventSource)
    render(<LogProbe url="http://test/logs" />)
    const source = latestSource()

    act(() => {
      source.emitOpen()
      source.emitMessage(logEvent('line 1'))
    })

    // Resent burst is longer than what's buffered (one line): the
    // matching first line is dropped, then the rest is genuinely new
    // and must all still be appended, not lost.
    act(() => {
      source.emitOpen()
      source.emitMessage(logEvent('line 1'))
      source.emitMessage(logEvent('line 2'))
      source.emitMessage(logEvent('line 3'))
    })

    screen.getByText('lines:line 1|line 2|line 3')
  })
})
