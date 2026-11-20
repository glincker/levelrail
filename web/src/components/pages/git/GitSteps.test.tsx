import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { GitSteps } from './GitSteps'
import type { GitStep } from './pagesGitLogic'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

describe('GitSteps', () => {
  it('renders every state with an accessible label', () => {
    const steps: GitStep[] = [
      { name: 'cloning', state: 'done' },
      { name: 'building', state: 'running' },
      { name: 'publishing', state: 'failed' },
      { name: 'live', state: 'unknown' },
    ]
    render(<GitSteps steps={steps} />)
    const items = screen.getAllByRole('listitem')
    expect(items.map((i) => i.getAttribute('data-state'))).toEqual([
      'done',
      'running',
      'failed',
      'unknown',
    ])
    expect(screen.getByText('git.steps.state.failed')).toBeTruthy()
  })
})
