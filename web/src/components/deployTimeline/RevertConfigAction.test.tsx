import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { RevertConfigAction } from './RevertConfigAction'

const mutate = vi.fn()

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}))
vi.mock('../../queries/appConfigRevisions', () => ({
  useRevertConfig: () => ({
    mutate,
    reset: vi.fn(),
    isPending: false,
    error: null,
  }),
}))

describe('RevertConfigAction', () => {
  it('previews with a dry run, then reverts on confirm', async () => {
    mutate.mockImplementationOnce(
      (_vars: unknown, opts: { onSuccess: (r: unknown) => void }) => {
        opts.onSuccess({
          changes: [{ kind: 'env', field: 'MODE', from: 'broken', to: 'prod' }],
        })
      },
    )
    render(<RevertConfigAction appName="web" revision={3} />)
    fireEvent.click(screen.getByText('revertConfig.action'))
    expect(mutate.mock.calls[0]?.[0]).toEqual({ to: 3, dryRun: true })
    await waitFor(() => {
      expect(screen.getByText('revertConfig.disclaimer')).toBeTruthy()
    })
    expect(screen.getByText('env MODE: broken > prod')).toBeTruthy()
    fireEvent.click(screen.getByText('revertConfig.confirm'))
    expect(mutate.mock.calls[1]?.[0]).toEqual({ to: 3, dryRun: false })
  })
})
