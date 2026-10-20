import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../../test/databasesI18n'
import type { DatabaseResource } from '../../types/databaseDetail'
import { ConnectionDetailsCard } from './ConnectionDetailsCard'

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

const db = {
  name: 'orders-db',
  engine: 'postgres',
  version: '16',
  tls_enabled: true,
} as DatabaseResource

function renderCard(database: DatabaseResource) {
  return render(
    <DatabasesI18n>
      <ConnectionDetailsCard database={database} />
    </DatabasesI18n>,
  )
}

describe('ConnectionDetailsCard', () => {
  it('explains private-only reachability and links to Access for credentials', () => {
    renderCard(db)
    expect(screen.getByText('Private network only')).toBeInTheDocument()
    expect(screen.getByText('TLS enabled')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Manage access' }),
    ).toBeInTheDocument()
  })

  it('shows a copyable address when public access is on', () => {
    renderCard({ ...db, publicly_accessible: true, public_port: 5433 })
    expect(
      screen.getByRole('button', { name: /Copy Address/i }),
    ).toBeInTheDocument()
    expect(screen.getByText(/:5433$/)).toBeInTheDocument()
  })
})
