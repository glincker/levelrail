import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import DeployFailTail from './DeployFailTail'

const h: { lines: { step?: string; stream: string; line: string }[] } =
  vi.hoisted(() => ({ lines: [] }))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: { step?: string }) => (o?.step ? `${k}:${o.step}` : k),
  }),
}))
vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: { lines: h.lines } }),
}))
vi.mock('../../queries/pagesDeployLog', () => ({
  pagesDeployLogQueryOptions: () => ({}),
}))

describe('DeployFailTail', () => {
  it('shows the failed phase, the last lines and one fix hint', () => {
    h.lines = [
      { step: 'cloning', stream: 'stdout', line: 'cloned' },
      {
        step: 'building',
        stream: 'stdout',
        line: 'npm ERR! Missing script: "build"',
      },
      { step: 'building', stream: 'stderr', line: 'exit status 1' },
    ]
    render(<DeployFailTail name="s" deployId="d" lines={2} scan={10} />)
    expect(screen.getByText('logs.failedAt:logs.step.building')).toBeTruthy()
    expect(screen.queryByText('cloned')).toBeNull()
    expect(screen.getByRole('note').textContent).toBe(
      'logs.hint.missingBuildCommand',
    )
  })

  it('renders log text as text, never markup', () => {
    h.lines = [
      {
        step: 'building',
        stream: 'stderr',
        line: '<img src=x onerror=alert(1)>',
      },
    ]
    const { container } = render(
      <DeployFailTail name="s" deployId="d" lines={5} scan={10} />,
    )
    expect(container.querySelector('img')).toBeNull()
    expect(screen.queryByRole('note')).toBeNull()
  })
})
