import { describe, expect, it, vi } from 'vitest'
import { DEMO_INSTALL_EVENT, openDemoInstall } from '@/lib/installPrompt'

describe('openDemoInstall', () => {
  it('opens docs only when no listener claims the event', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    openDemoInstall('https://docs.example.test')
    expect(open).toHaveBeenCalledTimes(1)
    const claim = (e: Event) => {
      e.preventDefault()
    }
    window.addEventListener(DEMO_INSTALL_EVENT, claim)
    openDemoInstall('https://docs.example.test')
    window.removeEventListener(DEMO_INSTALL_EVENT, claim)
    expect(open).toHaveBeenCalledTimes(1)
    open.mockRestore()
  })
})
