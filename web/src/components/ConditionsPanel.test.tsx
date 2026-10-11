import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../test/databasesI18n'
import type { ReconcileCondition } from '../types/deploy'
import { ConditionsPanel } from './ConditionsPanel'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

const ready: ReconcileCondition = {
  Type: 'Ready',
  Status: 'True',
  Reason: 'AlreadyRunning',
  Message: 'container abc is running',
  LastTransitionTime: '2026-10-01T00:00:00Z',
}

describe('ConditionsPanel', () => {
  it('shows human copy and keeps the raw condition inside Technical details', () => {
    const { container } = render(
      <DatabasesI18n>
        <ConditionsPanel conditions={[ready]} />
      </DatabasesI18n>,
    )
    expect(screen.getByText('Already running')).toBeInTheDocument()
    expect(screen.getByText('OK')).toBeInTheDocument()
    const details = container.querySelector('details')
    expect(details).not.toBeNull()
    expect(
      within(details as HTMLElement).getByText('Ready: AlreadyRunning (True)'),
    ).toBeInTheDocument()
    expect(screen.queryByText('Ready: AlreadyRunning')).toBeNull()
  })

  it('explains the credentials case with a settings link', () => {
    render(
      <DatabasesI18n>
        <ConditionsPanel
          conditions={[
            {
              ...ready,
              Status: 'False',
              Reason: 'CredentialsNotConfigured',
            },
          ]}
        />
      </DatabasesI18n>,
    )
    expect(screen.getByText('Failing')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Configure it in Settings/ }),
    ).toBeInTheDocument()
  })

  it('has a real empty state', () => {
    render(
      <DatabasesI18n>
        <ConditionsPanel conditions={[]} />
      </DatabasesI18n>,
    )
    expect(
      screen.getByText('Nothing has been checked yet.'),
    ).toBeInTheDocument()
  })
})
