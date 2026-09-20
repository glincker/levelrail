import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AiChatMessageList } from './AiChatMessageList'
import type { AiChatMessage } from '../types/aiAssistant'

function messages(): AiChatMessage[] {
  return [
    {
      id: 'msg_1',
      role: 'user',
      content: 'Why did the deploy fail?',
      tool_calls: null,
      created_at: '2026-09-20T00:00:00.000Z',
    },
    {
      id: 'msg_2',
      role: 'assistant',
      content: 'Checking the last deploy attempt now.',
      tool_calls: [
        {
          confirmation_id: 'conf_1',
          tool_name: 'get_deploy_logs',
          arguments: { app: 'demo-app' },
          read_only: true,
          result: 'exit code 1: missing DATABASE_URL',
        },
        {
          confirmation_id: 'conf_2',
          tool_name: 'rollback_deploy',
          arguments: { app: 'demo-app' },
          read_only: false,
        },
      ],
      created_at: '2026-09-20T00:00:01.000Z',
    },
  ]
}

describe('AiChatMessageList', () => {
  it('renders user and assistant messages, and tool calls attached to the assistant message', () => {
    render(
      <AiChatMessageList
        messages={messages()}
        streaming={false}
        onApproveToolCall={vi.fn()}
        onRejectToolCall={vi.fn()}
      />,
    )

    expect(screen.getByText('Why did the deploy fail?')).toBeInTheDocument()
    expect(
      screen.getByText('Checking the last deploy attempt now.'),
    ).toBeInTheDocument()

    // Read-only tool call already auto-executed: lightweight note, result shown.
    expect(screen.getByText('get_deploy_logs')).toBeInTheDocument()
    expect(
      screen.getByText('exit code 1: missing DATABASE_URL'),
    ).toBeInTheDocument()

    // Mutating tool call still pending: full confirmation card with actions.
    expect(screen.getByText('rollback_deploy')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /approve/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /reject/i })).toBeInTheDocument()
  })

  it('renders a tool-role message as a lightweight system row', () => {
    render(
      <AiChatMessageList
        messages={[
          {
            id: 'msg_3',
            role: 'tool',
            content: 'get_deploy_logs returned 42 lines',
            tool_calls: null,
            created_at: '2026-09-20T00:00:02.000Z',
          },
        ]}
        streaming={false}
        onApproveToolCall={vi.fn()}
        onRejectToolCall={vi.fn()}
      />,
    )

    expect(
      screen.getByText('get_deploy_logs returned 42 lines'),
    ).toBeInTheDocument()
  })

  it('omits the content bubble for an empty in-flight assistant message', () => {
    render(
      <AiChatMessageList
        messages={[
          {
            id: 'msg_4',
            role: 'assistant',
            content: '',
            tool_calls: [],
            created_at: '2026-09-20T00:00:03.000Z',
          },
        ]}
        streaming
        onApproveToolCall={vi.fn()}
        onRejectToolCall={vi.fn()}
      />,
    )

    expect(
      document.querySelectorAll('[class*="whitespace-pre-wrap"]').length,
    ).toBe(0)
  })
})
