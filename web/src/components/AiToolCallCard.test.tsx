import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AiToolCallCard } from './AiToolCallCard'
import { resolveAiToolConfirmation } from '../queries/aiAssistant'
import type { AiToolCall } from '../types/aiAssistant'

function requestUrlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function fakeStreamResponse(status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    body: null,
    json: () => Promise.resolve({}),
  } as unknown as Response
}

function mockFetch(): ReturnType<
  typeof vi.fn<
    (input: RequestInfo | URL, init: RequestInit) => Promise<Response>
  >
> {
  const fetchMock = vi.fn<
    (input: RequestInfo | URL, init: RequestInit) => Promise<Response>
  >(() => Promise.resolve(fakeStreamResponse()))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function mutatingToolCall(): AiToolCall {
  return {
    confirmation_id: 'conf_1',
    tool_name: 'restart_app',
    arguments: { app: 'demo-app' },
    read_only: false,
  }
}

describe('AiToolCallCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('shows Approve/Reject for an unresolved mutating tool call', () => {
    render(
      <AiToolCallCard
        toolCall={mutatingToolCall()}
        onApprove={() => {}}
        onReject={() => {}}
      />,
    )

    expect(screen.getByText('restart_app')).toBeInTheDocument()
    expect(screen.getByText(/confirmation required/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /approve/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /reject/i })).toBeInTheDocument()
  })

  it('approving calls the confirmations endpoint with approve: true', async () => {
    const fetchMock = mockFetch()
    const user = userEvent.setup()
    const toolCall = mutatingToolCall()

    render(
      <AiToolCallCard
        toolCall={toolCall}
        onApprove={() => {
          void resolveAiToolConfirmation(
            'sess_1',
            toolCall.confirmation_id,
            true,
            () => {},
          )
        }}
        onReject={() => {
          void resolveAiToolConfirmation(
            'sess_1',
            toolCall.confirmation_id,
            false,
            () => {},
          )
        }}
      />,
    )

    await user.click(screen.getByRole('button', { name: /approve/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })
    const [url, init] = fetchMock.mock.calls[0]!
    expect(requestUrlOf(url)).toBe(
      '/api/v1/ai/sessions/sess_1/confirmations/conf_1',
    )
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toEqual({ approve: true })
  })

  it('rejecting calls the confirmations endpoint with approve: false', async () => {
    const fetchMock = mockFetch()
    const user = userEvent.setup()
    const toolCall = mutatingToolCall()

    render(
      <AiToolCallCard
        toolCall={toolCall}
        onApprove={() => {
          void resolveAiToolConfirmation(
            'sess_1',
            toolCall.confirmation_id,
            true,
            () => {},
          )
        }}
        onReject={() => {
          void resolveAiToolConfirmation(
            'sess_1',
            toolCall.confirmation_id,
            false,
            () => {},
          )
        }}
      />,
    )

    await user.click(screen.getByRole('button', { name: /reject/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })
    const [, init] = fetchMock.mock.calls[0]!
    expect(JSON.parse(init.body as string)).toEqual({ approve: false })
  })

  it('renders a lightweight note, no buttons, for an already-executed read-only tool call', () => {
    render(
      <AiToolCallCard
        toolCall={{
          confirmation_id: 'conf_2',
          tool_name: 'get_logs',
          arguments: { app: 'demo-app' },
          read_only: true,
          result: 'no errors found',
        }}
        onApprove={() => {}}
        onReject={() => {}}
      />,
    )

    expect(screen.getByText(/checked/i)).toBeInTheDocument()
    expect(screen.getByText('get_logs')).toBeInTheDocument()
    expect(screen.getByText('no errors found')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /approve/i }),
    ).not.toBeInTheDocument()
  })

  it('renders a rejected note with no buttons once resolved', () => {
    render(
      <AiToolCallCard
        toolCall={{
          confirmation_id: 'conf_3',
          tool_name: 'rollback_deploy',
          arguments: {},
          read_only: false,
          resolution: 'rejected',
        }}
        onApprove={() => {}}
        onReject={() => {}}
      />,
    )

    expect(screen.getByText(/rejected/i)).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /approve/i }),
    ).not.toBeInTheDocument()
  })
})
