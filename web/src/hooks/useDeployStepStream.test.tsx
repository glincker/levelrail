import { act } from 'react'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, it, vi } from 'vitest'
import { useDeployStepStream } from './useDeployStepStream'

// Same stub EventSource shape useLogStream.test.tsx already establishes
// for this exact "jsdom has no native EventSource" gap.
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
    // no-op
  }

  emitOpen(): void {
    this.onopen?.()
  }

  emitMessage(data: string): void {
    this.onmessage?.({ data } as MessageEvent<string>)
  }
}

function stepEvent(
  step: string,
  status: 'running' | 'done' | 'failed',
): string {
  return JSON.stringify({ step, status, timestamp: '2026-09-23T00:00:00Z' })
}

function latestSource(): StubEventSource {
  const source = StubEventSource.instances[StubEventSource.instances.length - 1]
  if (!source) throw new Error('no StubEventSource was constructed')
  return source
}

function StepProbe({ url }: { url: string }) {
  const { steps, connectionState } = useDeployStepStream(url)
  return (
    <div>
      <p>state:{connectionState}</p>
      <p>steps:{steps.map((s) => `${s.step}:${s.status}`).join('|')}</p>
    </div>
  )
}

describe('useDeployStepStream', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    StubEventSource.instances = []
  })

  it('appends step events as they arrive', () => {
    vi.stubGlobal('EventSource', StubEventSource)
    render(<StepProbe url="http://test/steps" />)
    const source = latestSource()

    act(() => {
      source.emitOpen()
      source.emitMessage(stepEvent('detecting', 'running'))
    })

    screen.getByText('state:open')
    screen.getByText('steps:detecting:running')
  })

  it('upserts a later event for the same step in place, rather than appending a second row', () => {
    vi.stubGlobal('EventSource', StubEventSource)
    render(<StepProbe url="http://test/steps" />)
    const source = latestSource()

    act(() => {
      source.emitMessage(stepEvent('detecting', 'running'))
      source.emitMessage(stepEvent('detecting', 'done'))
      source.emitMessage(stepEvent('building', 'running'))
    })

    screen.getByText('steps:detecting:done|building:running')
  })

  it('drops a malformed payload without throwing', () => {
    vi.stubGlobal('EventSource', StubEventSource)
    render(<StepProbe url="http://test/steps" />)
    const source = latestSource()

    act(() => {
      source.emitMessage('not json')
      source.emitMessage(stepEvent('building', 'running'))
    })

    screen.getByText('steps:building:running')
  })
})
