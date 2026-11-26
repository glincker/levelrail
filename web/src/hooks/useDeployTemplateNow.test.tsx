import { renderHook, act } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { ApiError } from '../lib/apiError'

const { add } = vi.hoisted(() => ({ add: vi.fn() }))
vi.mock('@/components/ui/toast', () => ({ toast: { add } }))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
const { mutate } = vi.hoisted(() => ({ mutate: vi.fn() }))
vi.mock('../queries/serviceTemplates', () => ({
  useDeployServiceTemplateNow: () => ({ mutate, isPending: false }),
}))

import { useDeployTemplateNow } from './useDeployTemplateNow'

describe('useDeployTemplateNow', () => {
  beforeEach(() => {
    add.mockClear()
  })

  function fail(error: ApiError) {
    mutate.mockImplementation(
      (_id: string, o: { onError: (e: ApiError) => void }) => o.onError(error),
    )
    const { result } = renderHook(() => useDeployTemplateNow())
    act(() => {
      result.current.deploy('x')
    })
  }

  it('skips the failure toast for the demo read-only block', () => {
    fail(new ApiError(403, 'read-only', { code: 'demo_readonly' }))
    expect(add).not.toHaveBeenCalled()
  })

  it('still toasts other failures', () => {
    fail(new ApiError(500, 'boom'))
    expect(add).toHaveBeenCalledTimes(1)
  })
})
