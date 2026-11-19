import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SiteSettingsForm } from './SiteSettingsForm'
import type { SiteSettings } from '../../queries/pagesSites'

const h = vi.hoisted(() => ({
  data: undefined as SiteSettings | undefined,
  mutate: vi.fn(),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: { value?: string }) =>
      o?.value ? `${k}:${o.value}` : k,
  }),
}))
vi.mock('@tanstack/react-query', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-query')>()),
  useQuery: () => ({ data: undefined }),
}))
vi.mock('../../queries/pagesSites', async (orig) => ({
  ...(await orig<typeof import('../../queries/pagesSites')>()),
  useSiteSettings: () => ({ isPending: false, isError: false, data: h.data }),
  useUpdateSiteSettings: () => ({
    mutate: h.mutate,
    isPending: false,
    isError: false,
  }),
}))
vi.mock('../../queries/gitSources', () => ({
  useGitSource: () => ({
    data: { build_command: 'npm run build', publish_dir: 'out' },
  }),
}))
vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const settings: SiteSettings = {
  name: 's',
  build_command: '',
  output_dir: 'dist',
  spa_fallback: null,
  clean_urls: true,
  trailing_slash: '',
  not_found_page: '',
  protected: false,
  effective: { spa: false, clean_urls: true },
}

beforeEach(() => {
  h.data = settings
  h.mutate.mockReset()
})

describe('SiteSettingsForm', () => {
  it('captions build fields that come from git settings', () => {
    render(<SiteSettingsForm name="s" />)
    expect(screen.getByText('settings.fromGit:npm run build')).toBeTruthy()
    expect(screen.queryByText('settings.fromGit:out')).toBeNull()
  })

  it('shows null as Auto with the effective caption', () => {
    render(<SiteSettingsForm name="s" />)
    const spa = screen.getByRole('radiogroup', { name: 'settings.spa' })
    expect(spa.querySelector('input:checked')?.getAttribute('value')).toBe(
      'auto',
    )
    expect(screen.getByText('settings.current:settings.off')).toBeTruthy()
    expect(screen.getByText('settings.current:settings.on')).toBeTruthy()
  })

  it('saves a changed field and only that field', () => {
    render(<SiteSettingsForm name="s" />)
    fireEvent.change(screen.getByLabelText('settings.build'), {
      target: { value: 'npm run build' },
    })
    fireEvent.click(screen.getByText('settings.save'))
    expect(h.mutate.mock.calls[0]?.[0]).toEqual({
      build_command: 'npm run build',
    })
  })

  it('resets an override back to app.yaml', () => {
    render(<SiteSettingsForm name="s" />)
    // clean_urls and output_dir are overridden, so two reset buttons show
    const resets = screen.getAllByText('settings.reset')
    expect(resets).toHaveLength(1)
    expect(screen.getByText('settings.resetGit')).toBeTruthy()
    fireEvent.click(resets[0] as HTMLElement)
    expect(h.mutate.mock.calls[0]?.[0]).toEqual({ reset: ['clean_urls'] })
  })

  it('saves a new password and clears it from the field', () => {
    render(<SiteSettingsForm name="s" />)
    fireEvent.click(screen.getByRole('switch', { name: 'settings.password' }))
    fireEvent.change(screen.getByLabelText('settings.passwordPlaceholder'), {
      target: { value: 'correct horse' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'settings.save' }))
    expect(h.mutate).toHaveBeenCalledWith(
      { password: 'correct horse' },
      expect.anything(),
    )
  })

  it('asks before clearing and restores the switch when the clear fails', () => {
    h.data = { ...settings, protected: true }
    render(<SiteSettingsForm name="s" />)
    fireEvent.click(screen.getByRole('switch', { name: 'settings.password' }))
    expect(h.mutate).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: 'settings.passwordKeep' }),
    )
    expect(
      screen
        .getByRole('switch', { name: 'settings.password' })
        .getAttribute('aria-checked'),
    ).toBe('true')
    fireEvent.click(screen.getByRole('switch', { name: 'settings.password' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'settings.passwordClear' }),
    )
    expect(h.mutate).toHaveBeenCalledWith({ password: '' }, expect.anything())
    const opts = h.mutate.mock.calls[0]?.[1] as { onError: (e: Error) => void }
    act(() => opts.onError(new Error('nope')))
    expect(
      screen
        .getByRole('switch', { name: 'settings.password' })
        .getAttribute('aria-checked'),
    ).toBe('true')
  })
})
