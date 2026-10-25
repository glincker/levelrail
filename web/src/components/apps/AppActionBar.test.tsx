import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { AppDetail } from '../../types/appDetail'
import { AppActionBar } from './AppActionBar'

const restart = vi.fn()
vi.mock('../overview/useOverviewActions', () => ({
  useOverviewActions: () => ({
    redeploy: vi.fn(),
    redeploying: false,
    restart,
    restarting: false,
    toggleRunning: vi.fn(),
    togglingRunning: false,
    openApp: vi.fn(),
    copyUrl: vi.fn(),
    copied: false,
    goToDeploys: vi.fn(),
  }),
}))
const wakeMutate = vi.fn()
const sleepState = { asleep: false }
vi.mock('../../queries/appSleep', () => ({
  useAppAsleep: () => sleepState.asleep,
  useAppSleepActions: () => ({
    wake: { mutate: wakeMutate, isPending: false },
  }),
}))
const maintenanceState = { on: false }
vi.mock('../../hooks/useAppInMaintenance', () => ({
  useAppInMaintenance: () => maintenanceState.on,
}))
const gitState: { source: { repo_url: string; branch: string } | undefined } = {
  source: undefined,
}
vi.mock('../../queries/gitSources', () => ({
  useGitSource: () => ({ data: gitState.source }),
}))
vi.mock('../newapp/DeployLinkDialog', () => ({
  DeployLinkDialog: ({ repo }: { repo: string }) => <p>link for {repo}</p>,
}))
vi.mock('../PromoteAppDialog', () => ({ PromoteAppDialog: () => null }))
vi.mock('../CloneAppDialog', () => ({ CloneAppDialog: () => null }))
vi.mock('../DeleteAppDialog', () => ({ DeleteAppDialog: () => null }))

const app = { name: 'web', suspended: false } as AppDetail

describe('AppActionBar', () => {
  it('shows one primary Deploy, Restart, and a single overflow trigger', () => {
    render(<AppActionBar app={app} onDeploy={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'Deploy' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Restart' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'More actions' }),
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /delete/i })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Clone' })).toBeNull()
  })

  it('opens the deploy sheet on the image tab from Deploy', async () => {
    const onDeploy = vi.fn()
    render(<AppActionBar app={app} onDeploy={onDeploy} />)
    await userEvent.click(screen.getByRole('button', { name: 'Deploy' }))
    expect(onDeploy).toHaveBeenCalledWith('existing-image')
  })

  it('keeps Stop, Clone and Delete inside the overflow menu', async () => {
    render(<AppActionBar app={app} onDeploy={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    for (const name of ['Stop', 'Clone...', 'Delete...']) {
      expect(await screen.findByText(name)).toBeInTheDocument()
    }
  })

  it('offers Wake now instead of Start or Stop for a sleeping app', async () => {
    sleepState.asleep = true
    render(<AppActionBar app={app} onDeploy={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    await userEvent.click(await screen.findByText('Wake now'))
    expect(wakeMutate).toHaveBeenCalled()
    sleepState.asleep = false
  })

  it('offers the Deploy button link only for git-sourced apps', async () => {
    render(<AppActionBar app={app} onDeploy={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    await screen.findByText('Delete...')
    expect(
      screen.queryByText(/deployLinkDialog\.action|Deploy button link/),
    ).toBeNull()
  })

  it('opens the Deploy button link dialog with the app repo', async () => {
    gitState.source = { repo_url: 'https://github.com/you/web', branch: 'main' }
    render(<AppActionBar app={app} onDeploy={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    await userEvent.click(
      await screen.findByText(/deployLinkDialog\.action|Deploy button link/),
    )
    expect(
      await screen.findByText('link for https://github.com/you/web'),
    ).toBeInTheDocument()
    gitState.source = undefined
  })
})
