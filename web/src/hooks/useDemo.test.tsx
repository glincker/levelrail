import { act, render, renderHook, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { Button } from '@/components/ui/button'
import { setDemoInfo } from '@/lib/demoStore'
import { useIsDemo } from './useDemo'

afterEach(() => {
  setDemoInfo({ enabled: false })
})

describe('useIsDemo', () => {
  it('follows the demo store', () => {
    const { result } = renderHook(() => useIsDemo())
    expect(result.current).toBe(false)
    act(() => {
      setDemoInfo({ enabled: true })
    })
    expect(result.current).toBe(true)
  })
})

describe('Button writeAction', () => {
  it('stays enabled outside the demo', () => {
    render(<Button writeAction>Deploy</Button>)
    expect(screen.getByRole('button', { name: 'Deploy' })).not.toBeDisabled()
  })

  it('is disabled in the demo', () => {
    setDemoInfo({ enabled: true })
    render(<Button writeAction>Deploy</Button>)
    expect(screen.getByRole('button', { name: 'Deploy' })).toBeDisabled()
  })

  it('leaves non-write buttons alone in the demo', () => {
    setDemoInfo({ enabled: true })
    render(<Button>Open</Button>)
    expect(screen.getByRole('button', { name: 'Open' })).not.toBeDisabled()
  })
})
