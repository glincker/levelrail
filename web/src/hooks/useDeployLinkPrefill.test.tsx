import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useDeployLinkPrefill } from './useDeployLinkPrefill'

describe('useDeployLinkPrefill', () => {
  it('returns prefilled values for a valid link', () => {
    const { result } = renderHook(() =>
      useDeployLinkPrefill({
        deploy: '1',
        image: 'nginx:1.27',
        port: '8080',
        env: 'API_KEY',
      }),
    )
    expect(result.current).toMatchObject({
      active: true,
      image: 'nginx:1.27',
      port: 8080,
      env: ['API_KEY'],
      name: 'nginx',
      ignored: [],
    })
  })

  it('stays inactive and empty without deploy=1', () => {
    const { result } = renderHook(() =>
      useDeployLinkPrefill({ image: 'nginx' }),
    )
    expect(result.current.active).toBe(false)
    expect(result.current.image).toBeUndefined()
  })

  it('reports ignored params and keeps a stable reference for equal input', () => {
    const { result, rerender } = renderHook(
      (s: Record<string, string>) => useDeployLinkPrefill(s),
      { initialProps: { deploy: '1', image: 'nginx', port: '0' } },
    )
    const first = result.current
    expect(first.ignored).toEqual(['port'])
    rerender({ deploy: '1', image: 'nginx', port: '0' })
    expect(result.current).toBe(first)
  })
})
