import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../test/databasesI18n'
import type { DatabaseListEntry } from '../types/databaseDetail'
import { DatabaseRow } from './DatabaseRow'

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

function makeDb(over: Partial<DatabaseListEntry>): DatabaseListEntry {
  return {
    name: 'orders-db',
    engine: 'postgres',
    version: '16',
    status: { label: 'Healthy', variant: 'success' },
    ...over,
  }
}

describe('DatabaseRow without a container', () => {
  it('labels a pending record and hides the running status', () => {
    render(
      <DatabasesI18n>
        <DatabaseRow database={makeDb({ provision_state: 'pending' })} />
      </DatabasesI18n>,
    )
    expect(screen.getByText('Not created yet')).toBeTruthy()
    expect(screen.queryByText('Healthy')).toBeNull()
  })

  it('labels a failed record with the reason', () => {
    render(
      <DatabasesI18n>
        <DatabaseRow
          database={makeDb({
            provision_state: 'failed',
            provision_reason: 'no space left',
          })}
        />
      </DatabasesI18n>,
    )
    const label = screen.getByText('Failed to create')
    expect(label.getAttribute('title')).toBe('no space left')
    expect(screen.queryByText('Healthy')).toBeNull()
  })
})
