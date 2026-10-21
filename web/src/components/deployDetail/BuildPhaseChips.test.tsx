import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { LogLine } from '../../hooks/useLogStream'
import { analyzeBuildLog } from '../../lib/buildLogPhases'
import { DeployDetailI18n } from '@/test/deployDetailI18n'
import { BuildPhaseChips } from './BuildPhaseChips'

function toLines(texts: string[]): LogLine[] {
  return texts.map((line, i) => ({ id: i + 1, line, stream: 'stdout' }))
}

const sample = analyzeBuildLog(
  toLines([
    '#1 [internal] load build definition',
    '#2 [2/4] RUN npm ci',
    '#2 DONE 10.0s',
    '#3 [3/4] RUN npm run build',
    '#3 DONE 5.0s',
    '#4 exporting to image',
  ]),
)

describe('BuildPhaseChips', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders counted chips with aria-pressed and calls onSelect', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(
      <DeployDetailI18n>
        <BuildPhaseChips analysis={sample} active="all" onSelect={onSelect} />
      </DeployDetailI18n>,
    )
    expect(screen.getByRole('button', { name: 'All (6)' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    const install = screen.getByRole('button', { name: 'Install (2)' })
    expect(install).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: 'Build (2)' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Push (1)' })).toBeEnabled()
    install.focus()
    await user.keyboard('{Enter}')
    expect(onSelect).toHaveBeenCalledWith('install')
  })

  it('is hidden when no phase is recognised', () => {
    const none = analyzeBuildLog(toLines(['hello', 'world']))
    const { container } = render(
      <DeployDetailI18n>
        <BuildPhaseChips analysis={none} active="all" onSelect={vi.fn()} />
      </DeployDetailI18n>,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
