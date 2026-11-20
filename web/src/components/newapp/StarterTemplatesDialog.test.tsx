import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '../../lib/apiError'
import { NewAppI18n } from '../../test/newAppI18n'
import { StarterTemplatesDialog } from './StarterTemplatesDialog'

vi.mock('../ApiErrorAlert', () => ({
  ApiErrorAlert: ({ error }: { error: unknown }) => (
    <div data-testid="api-error-alert">
      {error instanceof ApiError ? `${error.code}:${error.message}` : 'other'}
    </div>
  ),
}))

vi.mock('../../queries/projectTemplates', () => ({
  ProjectTemplateValidationError: class extends Error {
    fields: { param: string; message: string }[] = []
  },
  useProjectTemplates: () => ({
    isError: true,
    data: undefined,
    error: new ApiError(503, 'templates unavailable', { code: 'unavailable' }),
  }),
  useRenderProjectTemplate: () => ({
    error: null,
    isPending: false,
    reset: vi.fn(),
  }),
}))

describe('StarterTemplatesDialog', () => {
  it('renders a failed template load through ApiErrorAlert', async () => {
    render(
      <NewAppI18n>
        <StarterTemplatesDialog />
      </NewAppI18n>,
    )
    await userEvent.click(screen.getByRole('button'))
    expect(await screen.findByTestId('api-error-alert')).toHaveTextContent(
      'unavailable:templates unavailable',
    )
  })
})
