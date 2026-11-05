import { act } from 'react'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import observabilityEn from '../locales/en/observability.json'
import appRowsEn from '../locales/en/appRows.json'
import { parseRunStatus } from '../hooks/useLogStream'
import type { ReconcileCondition } from '../types/deploy'
import { LiveLogViewer } from './LiveLogViewer'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock('./LogTerminal', () => ({
  LogTerminal: ({ lines }: { lines: { line: string }[] }) => (
    <pre>{lines.map((l) => l.line).join('\n')}</pre>
  ),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['observability', 'appRows'],
  defaultNS: 'observability',
  resources: { en: { observability: observabilityEn, appRows: appRowsEn } },
  interpolation: { escapeValue: false },
})

type Listener = (event: MessageEvent<string>) => void

class StubEventSource {
  static instances: StubEventSource[] = []
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: Listener | null = null
  listeners: Record<string, Listener[]> = {}

  url: string

  constructor(url: string) {
    this.url = url
    StubEventSource.instances.push(this)
  }

  addEventListener(type: string, fn: Listener): void {
    ;(this.listeners[type] ??= []).push(fn)
  }

  close(): void {
    // nothing to release in the stub
  }

  emit(type: string, data: string): void {
    const event = { data } as MessageEvent<string>
    if (type === 'message') this.onmessage?.(event)
    for (const fn of this.listeners[type] ?? []) fn(event)
  }
}

function source(): StubEventSource {
  const s = StubEventSource.instances[StubEventSource.instances.length - 1]
  if (!s) throw new Error('no EventSource constructed')
  return s
}

function renderViewer(conditions?: ReconcileCondition[]) {
  vi.stubGlobal('EventSource', StubEventSource)
  render(
    <I18nextProvider i18n={testI18n}>
      <LiveLogViewer appName="web" conditions={conditions} />
    </I18nextProvider>,
  )
}

describe('LiveLogViewer', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    StubEventSource.instances = []
  })

  it('shows Live while the container runs and keeps streaming', () => {
    renderViewer()
    act(() => {
      source().onopen?.()
      source().emit('status', JSON.stringify({ running: true }))
      source().emit(
        'message',
        JSON.stringify({ line: 'listening on :80', stream: 'stdout' }),
      )
    })
    expect(screen.getByText('Live')).toBeInTheDocument()
    expect(screen.queryByText('Not running')).not.toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(
      screen.getByText(/1 lines \(recent context, then live\)/),
    ).toBeInTheDocument()
  })

  it('replaces Live with Not running and explains the exit', () => {
    renderViewer()
    act(() => {
      source().onopen?.()
      source().emit(
        'status',
        JSON.stringify({
          running: false,
          exit_code: 0,
          finished_at: '2026-10-14T12:00:00Z',
          reason: 'ExitedDuringReadiness',
          message:
            'Container exited with code 0 after 2s, before it became ready',
        }),
      )
      source().emit(
        'message',
        JSON.stringify({ line: 'usage: ntfy serve', stream: 'stdout' }),
      )
    })
    expect(screen.getByText('Not running')).toBeInTheDocument()
    expect(screen.queryByText('Live')).not.toBeInTheDocument()
    expect(
      screen.getByText(/The container exited with code 0 at/),
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'Container exited with code 0 after 2s, before it became ready',
      ),
    ).toBeInTheDocument()
    expect(
      screen.getByText(/the last output of the stopped container/),
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Waiting for log output...'),
    ).not.toBeInTheDocument()
  })

  it('says there is no container and offers Deploy when stopped with no output', () => {
    renderViewer()
    act(() => {
      source().emit('status', JSON.stringify({ running: false }))
    })
    expect(
      screen.getByText(/No container is running for this app/),
    ).toBeInTheDocument()
    expect(screen.getByText('Deploy it')).toBeInTheDocument()
  })

  it('says No container instead of Connecting when none exists', () => {
    renderViewer([
      {
        Type: 'Ready',
        Status: 'False',
        Reason: 'ImagePullFailed',
        Message: 'pull failed',
        LastTransitionTime: '2026-10-14T12:00:00Z',
      },
    ])
    expect(screen.getByText('No container')).toBeInTheDocument()
    expect(screen.queryByText('Connecting...')).not.toBeInTheDocument()
    expect(
      screen.queryByText('Waiting for log output...'),
    ).not.toBeInTheDocument()
    expect(screen.getByText('Show last deploy log')).toBeInTheDocument()
  })

  it('names an OOM kill', () => {
    renderViewer()
    act(() => {
      source().onopen?.()
      source().emit(
        'status',
        JSON.stringify({ running: false, exit_code: 137, oom_killed: true }),
      )
    })
    expect(
      screen.getByText(
        'The container was killed for exceeding its memory limit',
      ),
    ).toBeInTheDocument()
  })

  it('a running status after an exit clears the banner', () => {
    renderViewer()
    act(() => {
      source().onopen?.()
      source().emit('status', JSON.stringify({ running: false, exit_code: 1 }))
    })
    expect(screen.getByText('Not running')).toBeInTheDocument()
    act(() => {
      source().emit('status', JSON.stringify({ running: true }))
    })
    expect(screen.getByText('Live')).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })
})

describe('parseRunStatus', () => {
  it.each([
    ['{"running":true}', true],
    ['{"running":false,"exit_code":0}', false],
  ])('parses %s', (raw, running) => {
    expect(parseRunStatus(raw)?.running).toBe(running)
  })

  it.each(['not json', '{}', '{"running":"no"}', 'null'])(
    'ignores %s',
    (raw) => {
      expect(parseRunStatus(raw)).toBeNull()
    },
  )
})
