import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../lib/apiError'
import { ApiErrorAlert } from './ApiErrorAlert'

const mutateAsync = vi.fn().mockResolvedValue({})
vi.mock('../queries/fixit', () => ({
  useApplyFix: () => ({ mutateAsync, isPending: false, isError: false }),
}))
vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: () => Promise.resolve() }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: { id?: string }) => (o?.id ? `${k} ${o.id}` : k),
  }),
}))

describe('ApiErrorAlert', () => {
  afterEach(() => {
    cleanup()
    mutateAsync.mockClear()
  })

  it('runs the typed fix', async () => {
    const err = new ApiError(507, 'Out of disk', {
      code: 'disk_full',
      fix: { id: 'disk.reclaim', label: 'Free up disk space' },
    })
    render(<ApiErrorAlert error={err} />)
    await userEvent.click(
      screen.getByRole('button', { name: 'Free up disk space' }),
    )
    expect(mutateAsync).toHaveBeenCalledWith({ id: 'disk.reclaim' })
  })

  it('shows a request id and never the bare internal error text', () => {
    const err = new ApiError(500, 'internal error', {
      code: 'internal_error',
      requestId: 'req_abc',
    })
    render(<ApiErrorAlert error={err} />)
    expect(screen.queryByText('internal error')).toBeNull()
    expect(screen.getByText('apiError.requestId req_abc')).toBeTruthy()
  })

  it('offers a Docker re-check for docker_unavailable', () => {
    const err = new ApiError(503, 'Docker is not responding', {
      code: 'docker_unavailable',
    })
    render(<ApiErrorAlert error={err} />)
    expect(screen.getByText('apiError.recheckDocker')).toBeTruthy()
  })
})
