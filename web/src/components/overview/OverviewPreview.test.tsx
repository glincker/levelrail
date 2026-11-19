import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DeployAttempt } from '../../types/deployAttempt'
import OverviewPreview from './OverviewPreview'
import { pickPreviewAttempt } from './previewAttempt'

const usePreviewStatus = vi.hoisted(() => vi.fn())
const usePreviewHistory = vi.hoisted(() => vi.fn())
vi.mock('../../queries/preview', () => ({
  usePreviewStatus,
  usePreviewHistory,
  useCapturePreview: () => ({ mutate: vi.fn(), isPending: false }),
}))
vi.mock('../DeployPreviewThumb', () => ({
  DeployPreviewThumb: ({ deploymentId }: { deploymentId: string }) => (
    <div data-testid="thumb">{deploymentId}</div>
  ),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      typeof o?.when === 'string' ? `${k}:${o.when}` : k,
  }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))

function attempt(over: Partial<DeployAttempt>): DeployAttempt {
  return { id: 'd1', status: 'succeeded', ...over } as DeployAttempt
}
const status = (over: Record<string, unknown>) => ({
  data: { mode: 'screenshot', server_enabled: true, ...over },
})

function view(attempts: DeployAttempt[]) {
  return render(
    <OverviewPreview
      appName="web"
      attempts={attempts}
      url="https://web.test"
      onDeploy={vi.fn()}
    />,
  )
}

describe('OverviewPreview', () => {
  beforeEach(() => {
    usePreviewStatus.mockReturnValue(status({}))
    usePreviewHistory.mockReturnValue({ data: [] })
  })

  it('shows a slim empty state before any successful deploy', () => {
    view([attempt({ status: 'failed' })])
    expect(screen.getByText('preview.empty')).toBeTruthy()
    expect(screen.queryByTestId('thumb')).toBeNull()
  })

  it('renders the serving release thumb with the capture time and open link', () => {
    usePreviewHistory.mockReturnValue({
      data: [
        {
          deployment_id: 'd2',
          status: 'ok',
          captured_at: '2026-11-18T10:00:00Z',
        },
      ],
    })
    view([
      attempt({ id: 'd1', rollout_state: 'superseded' }),
      attempt({ id: 'd2', rollout_state: 'serving' }),
    ])
    expect(screen.getByTestId('thumb').textContent).toBe('d2')
    expect(screen.getByText(/preview.captured/)).toBeTruthy()
    expect(
      screen.getByText('preview.open').closest('a')?.getAttribute('href'),
    ).toBe('https://web.test')
    expect(screen.queryByText('preview.turnOn')).toBeNull()
  })

  it('offers Turn on screenshots when previews are off', () => {
    usePreviewStatus.mockReturnValue(status({ mode: 'off' }))
    view([attempt({})])
    expect(screen.getByText('preview.off')).toBeTruthy()
    expect(screen.getByText('preview.turnOn')).toBeTruthy()
    expect(screen.queryByTestId('thumb')).toBeNull()
  })

  it('explains a server-wide off switch without an action', () => {
    usePreviewStatus.mockReturnValue(status({ server_enabled: false }))
    view([attempt({})])
    expect(screen.getByText('preview.serverOff')).toBeTruthy()
    expect(screen.queryByText('preview.turnOn')).toBeNull()
  })

  it('offers screenshots from the metadata default and keeps the thumb', () => {
    usePreviewStatus.mockReturnValue(status({ mode: 'metadata' }))
    view([attempt({})])
    expect(screen.getByText('preview.turnOn')).toBeTruthy()
    expect(screen.getByTestId('thumb')).toBeTruthy()
  })
})

describe('pickPreviewAttempt', () => {
  it('falls back to the newest succeeded attempt', () => {
    expect(
      pickPreviewAttempt([
        attempt({ id: 'a', status: 'failed' }),
        attempt({ id: 'b' }),
      ])?.id,
    ).toBe('b')
  })
})

describe('OverviewPreview static site', () => {
  beforeEach(() => {
    usePreviewStatus.mockReturnValue(status({}))
    usePreviewHistory.mockReturnValue({ data: [] })
  })

  it('shows a sandboxed script-less frame and a viewport toggle', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({}))
    const { container } = render(
      <OverviewPreview
        appName="site"
        attempts={[attempt({})]}
        url="https://site.test"
        isStatic
        onDeploy={vi.fn()}
      />,
    )
    const frame = await waitFor(() => {
      const f = container.querySelector('iframe')
      expect(f).toBeTruthy()
      return f as HTMLIFrameElement
    })
    expect(frame.getAttribute('sandbox')).toBe('')
    expect(screen.getByText('preview.mobile')).toBeTruthy()
    expect(screen.queryByTestId('thumb')).toBeNull()
    vi.unstubAllGlobals()
  })

  it('falls back to a placeholder when the site cannot be reached', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('cert')))
    const { container } = render(
      <OverviewPreview
        appName="site"
        attempts={[attempt({})]}
        url="https://site.localhost:8443"
        isStatic
        onDeploy={vi.fn()}
      />,
    )
    await screen.findByText('preview.frameBlocked')
    expect(container.querySelector('iframe')).toBeNull()
    vi.unstubAllGlobals()
  })
})
