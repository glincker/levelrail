import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { LogLine } from '../../hooks/useLogStream'
import { analyzeBuildLog } from '../../lib/buildLogPhases'
import { DeployDetailI18n } from '@/test/deployDetailI18n'
import { BuildStepList } from './BuildStepList'

function toLines(texts: string[]): LogLine[] {
  return texts.map((line, i) => ({ id: i + 1, line, stream: 'stdout' }))
}

const analysis = analyzeBuildLog(
  toLines([
    '#3 [3/4] RUN npm run build',
    '#3 DONE 5.0s',
    '#2 [2/4] RUN npm ci',
    '#2 CACHED',
    '#4 exporting to image',
    '#4 DONE 61.0s',
  ]),
)

describe('BuildStepList', () => {
  afterEach(() => {
    cleanup()
  })

  it('is collapsed by default, shows phase totals, and expands in order', async () => {
    const user = userEvent.setup()
    render(
      <DeployDetailI18n>
        <BuildStepList analysis={analysis} onJump={vi.fn()} />
      </DeployDetailI18n>,
    )
    expect(screen.queryByText('RUN npm ci')).not.toBeInTheDocument()
    expect(screen.getByTestId('phase-total-build')).toHaveTextContent(
      'Build: 5.0s',
    )
    expect(screen.getByTestId('phase-total-install')).toHaveTextContent(
      'no timing',
    )
    expect(screen.getByTestId('phase-total-push')).toHaveTextContent(
      'Push: 1m 1s',
    )
    const toggle = screen.getByRole('button', { name: 'Show steps' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await user.click(toggle)
    const items = screen
      .getAllByRole('listitem')
      .filter((li) => li.querySelector('button[title]'))
    expect(items.map((li) => li.textContent)).toEqual([
      'RUN npm run buildBuild5.0s',
      'RUN npm ciInstallCached',
      'exporting to imagePush1m 1s',
    ])
  })

  it('jumps to the first line of the clicked step', async () => {
    const user = userEvent.setup()
    const onJump = vi.fn()
    render(
      <DeployDetailI18n>
        <BuildStepList analysis={analysis} onJump={onJump} />
      </DeployDetailI18n>,
    )
    await user.click(screen.getByRole('button', { name: 'Show steps' }))
    await user.click(screen.getByText('RUN npm ci'))
    expect(onJump).toHaveBeenCalledWith(
      expect.objectContaining({ id: '2', firstLine: 2 }),
    )
    expect(screen.getByText('Cached')).toBeInTheDocument()
  })

  it('renders nothing without recognised steps', () => {
    const { container } = render(
      <DeployDetailI18n>
        <BuildStepList
          analysis={analyzeBuildLog(toLines(['plain']))}
          onJump={vi.fn()}
        />
      </DeployDetailI18n>,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
