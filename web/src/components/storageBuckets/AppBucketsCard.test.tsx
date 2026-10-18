import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../../test/NewSurfacesProviders'
import { jsonResponse, newSurfacesI18n } from '../../test/newSurfacesI18n'
import type { AppDetail } from '../../types/appDetail'
import { AppBucketsCard } from './AppBucketsCard'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const bucket = {
  id: 'b1',
  name: 'uploads',
  target_id: 't1',
  prefix: 'uploads/',
  public: false,
  quota_bytes: 0,
  lifecycle_days: 0,
  created_at: '2026-10-01T00:00:00Z',
  apps: [{ app_name: 'web', env_prefix: 'S3' }],
  env_keys: ['S3_BUCKET'],
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AppBucketsCard', () => {
  it('shows bound buckets and asks before unbinding', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse([bucket]))),
    )
    render(
      <NewSurfacesProviders>
        <AppBucketsCard app={{ name: 'web' } as AppDetail} />
      </NewSurfacesProviders>,
    )
    expect(await screen.findByText('uploads')).toBeInTheDocument()
    expect(screen.getByText('S3_BUCKET')).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', {
        name: newSurfacesI18n.t('card.unbind', { ns: 'storageBuckets' }),
      }),
    )
    expect(
      await screen.findByText(
        newSurfacesI18n.t('card.unbindTitle', { ns: 'storageBuckets' }),
      ),
    ).toBeInTheDocument()
  })
})
