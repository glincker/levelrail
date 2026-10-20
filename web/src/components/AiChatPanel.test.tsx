import { render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AiChatPanel } from './AiChatPanel'
import { resetAiChatSeedForTests, setAiChatSeed } from '../lib/aiChatSeed'

const { sendMessage } = vi.hoisted(() => ({ sendMessage: vi.fn() }))
vi.mock('../hooks/useAiChatSession', () => ({
  useAiChatSession: () => ({
    sessionId: 'sess-1',
    messages: [],
    connectionState: 'ready',
    streaming: false,
    error: null,
    sendMessage,
    resolveConfirmation: vi.fn(),
    startNewSession: vi.fn(),
    resumeSession: vi.fn(),
  }),
}))
vi.mock('./AiChatHistoryMenu', () => ({ AiChatHistoryMenu: () => null }))
vi.mock('./AiChatComposer', () => ({ AiChatComposer: () => null }))
vi.mock('./AiChatMessageList', () => ({ AiChatMessageList: () => null }))

describe('AiChatPanel seeding', () => {
  beforeEach(() => {
    sendMessage.mockReset()
  })

  afterEach(() => {
    resetAiChatSeedForTests()
  })

  it('sends a pending dashboard seed once the session is ready', async () => {
    setAiChatSeed('Help me fix this: web is failing. Target: web.')
    render(<AiChatPanel />)
    await waitFor(() => {
      expect(sendMessage).toHaveBeenCalledWith(
        'Help me fix this: web is failing. Target: web.',
      )
    })
  })

  it('does nothing when there is no pending seed', async () => {
    render(<AiChatPanel />)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(sendMessage).not.toHaveBeenCalled()
  })
})
