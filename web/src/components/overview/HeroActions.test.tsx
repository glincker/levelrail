import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { AppDetail } from '../../types/appDetail'
import { HeroActions } from './HeroActions'

const redeploy = vi.fn()
vi.mock('./useOverviewActions', () => ({
  useOverviewActions: () => ({
    redeploy,
    redeploying: false,
    restart: vi.fn(),
    restarting: false,
    toggleRunning: vi.fn(),
    togglingRunning: false,
    openApp: vi.fn(),
    copyUrl: vi.fn(),
    copied: false,
    goToDeploys: vi.fn(),
  }),
}))
vi.mock('../PromoteAppDialog', () => ({ PromoteAppDialog: () => null }))
vi.mock('../CloneAppDialog', () => ({ CloneAppDialog: () => null }))
vi.mock('../DeleteAppDialog', () => ({ DeleteAppDialog: () => null }))

const app = { name: 'web', suspended: false } as AppDetail

describe('HeroActions', () => {
  it('has Open app as a link to the live URL', () => {
    render(
      <HeroActions app={app} url="https://a.example.com" onDeploy={vi.fn()} />,
    )
    const link = screen.getByRole('link', { name: /open app/i })
    expect(link).toHaveAttribute('href', 'https://a.example.com')
  })

  it('hides Open app without a URL', () => {
    render(<HeroActions app={app} url={null} onDeploy={vi.fn()} />)
    expect(screen.queryByRole('link', { name: /open app/i })).toBeNull()
  })

  it('opens the deploy sheet on the existing image tab from the main button', async () => {
    const onDeploy = vi.fn()
    render(<HeroActions app={app} url={null} onDeploy={onDeploy} />)
    await userEvent.click(screen.getByRole('button', { name: 'Deploy' }))
    expect(onDeploy).toHaveBeenCalledWith('existing-image')
  })

  it('offers redeploy, build from source and rollback in the split menu', async () => {
    const onDeploy = vi.fn()
    render(<HeroActions app={app} url={null} onDeploy={onDeploy} />)
    await userEvent.click(
      screen.getByRole('button', { name: 'More deploy options' }),
    )
    await userEvent.click(await screen.findByText('Build from source...'))
    expect(onDeploy).toHaveBeenCalledWith('build-from-source')

    await userEvent.click(
      screen.getByRole('button', { name: 'More deploy options' }),
    )
    await userEvent.click(await screen.findByText('Redeploy current'))
    expect(redeploy).toHaveBeenCalled()
  })

  it('keeps Delete inside the overflow menu, not as a top-level button', async () => {
    render(<HeroActions app={app} url={null} onDeploy={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /delete/i })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    expect(await screen.findByText('Delete...')).toBeInTheDocument()
    expect(screen.getByText('Restart')).toBeInTheDocument()
    expect(screen.getByText('Stop')).toBeInTheDocument()
  })
})
