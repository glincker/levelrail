import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { makeDeployment } from '../../test/deploymentFixtures'
import { DeploymentRow } from './DeploymentRow'
import type { Deployment } from '../../types/deployment'

const NOW = new Date('2026-09-26T10:01:00Z').getTime()

function renderRow(over: Partial<Deployment>, extra: { open?: boolean } = {}) {
  const onOpen = vi.fn()
  const d = makeDeployment(over)
  render(
    <DeploymentRow
      d={d}
      now={NOW}
      open={extra.open ?? false}
      focused={false}
      onOpen={onOpen}
    />,
  )
  return { d, onOpen, row: screen.getByRole('button') }
}

describe('DeploymentRow states', () => {
  it('shows a ready deploy with word, duration, commit, branch and app', () => {
    const { row } = renderRow({})
    expect(row).toHaveAttribute('data-kind', 'standard')
    expect(screen.getByText('Ready')).toBeInTheDocument()
    expect(screen.getByText('3m 31s')).toBeInTheDocument()
    expect(screen.getByText('Fix the login redirect')).toBeInTheDocument()
    expect(screen.getByText('1234567')).toBeInTheDocument()
    expect(screen.getByText('main')).toBeInTheDocument()
    expect(screen.getByText('web')).toBeInTheDocument()
  })

  it('highlights the current live production release', () => {
    const { row } = renderRow({ is_live: true })
    expect(row).toHaveAttribute('data-kind', 'live')
    expect(screen.getByTitle('Current production release')).toBeInTheDocument()
  })

  it('does not highlight a live preview release as production', () => {
    renderRow({ is_live: true, environment: 'preview' })
    expect(screen.queryByTitle('Current production release')).toBeNull()
  })

  it('shows a failed deploy with its reason and step', () => {
    const { row } = renderRow({
      status: 'failed',
      error_summary: 'build: exit status 1',
    })
    expect(row).toHaveAttribute('data-kind', 'failed')
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(screen.getByText('build: exit status 1')).toBeInTheDocument()
  })

  it('shows a redeploy instead of commit and branch', () => {
    const { row } = renderRow({
      commit_sha: '',
      branch: '',
      commit_message: '',
      trigger: 'manual',
    })
    expect(row).toHaveAttribute('data-kind', 'redeploy')
    expect(screen.getByText('Redeploy of web:1.2.3')).toBeInTheDocument()
    expect(screen.queryByText('main')).toBeNull()
  })

  it('shows a rollback target instead of commit and branch', () => {
    const { row } = renderRow({
      trigger: 'rollback',
      rollback_of: 'abcdef1234567890',
    })
    expect(row).toHaveAttribute('data-kind', 'rollback')
    expect(screen.getByText('Rollback to abcdef12')).toBeInTheDocument()
    expect(screen.queryByText('1234567')).toBeNull()
  })

  it('shows a superseded deploy muted with its winner', () => {
    const { row } = renderRow({
      status: 'superseded',
      superseded_by: 'ffff0000aaaa',
    })
    expect(row).toHaveAttribute('data-kind', 'superseded')
    expect(row).toHaveClass('opacity-70')
    expect(screen.getByText('Superseded by ffff0000')).toBeInTheDocument()
  })

  it('shows a queued deploy without a duration', () => {
    const { row } = renderRow({
      status: 'queued',
      duration_ms: null,
      finished_at: null,
      reason: 'Waiting for another deploy of this app',
    })
    expect(row).toHaveAttribute('data-kind', 'queued')
    expect(screen.getByText('Queued')).toBeInTheDocument()
    expect(
      screen.getByText('Waiting for another deploy of this app'),
    ).toBeInTheDocument()
  })

  it('ticks the duration and step progress of a building deploy from the clock', () => {
    renderRow({
      status: 'building',
      duration_ms: null,
      finished_at: null,
      steps: { done: 1, running: 1, failed: 0 },
    })
    expect(screen.getByText('Building')).toBeInTheDocument()
    expect(screen.getByText('1m 0s')).toBeInTheDocument()
    expect(screen.getByText('1/2 steps')).toBeInTheDocument()
  })

  it('opens the deployment on click and marks the open row', async () => {
    const { d, onOpen, row } = renderRow({}, { open: true })
    expect(row).toHaveAttribute('aria-current', 'true')
    await userEvent.click(row)
    expect(onOpen).toHaveBeenCalledWith(d.id)
  })
})

describe('DeploymentRow preview thumbnail', () => {
  const alt = 'Preview of the web deployment'

  it('shows a small thumbnail when a capture exists', () => {
    renderRow({ preview_image_url: '/api/v1/apps/web/deployments/d/preview' })
    expect(screen.getByAltText(alt)).toBeInTheDocument()
  })

  it('renders no thumbnail when there is no capture', () => {
    renderRow({ preview_image_url: null })
    expect(screen.queryByAltText(alt)).toBeNull()
  })

  it('drops the thumbnail when the image fails to load', () => {
    renderRow({ preview_image_url: '/broken.jpg' })
    fireEvent.error(screen.getByAltText(alt))
    expect(screen.queryByAltText(alt)).toBeNull()
  })
})
