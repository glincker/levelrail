import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useSuggestionRunner } from './useSuggestionRunner'

const { navigate, setAiChatSeed } = vi.hoisted(() => ({
  navigate: vi.fn(),
  setAiChatSeed: vi.fn(),
}))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => navigate }))
vi.mock('../../hooks/usePaletteAppActions', () => ({
  usePaletteAppActions: () => ({ restartApp: vi.fn(), redeployApp: vi.fn() }),
}))
vi.mock('../../lib/aiChatSeed', () => ({ setAiChatSeed }))

describe('useSuggestionRunner: ask-ai', () => {
  it('seeds the chat and navigates to the AI assistant', () => {
    const { result } = renderHook(() => useSuggestionRunner())
    act(() => {
      result.current({ kind: 'ask-ai', message: 'Help me fix this: web.' })
    })
    expect(setAiChatSeed).toHaveBeenCalledWith('Help me fix this: web.')
    expect(navigate).toHaveBeenCalledWith({ to: '/ai-assistant' })
  })
})
