import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../../../test/databasesI18n'
import type { DatabaseResource } from '../../../types/databaseDetail'
import { CacheConnectionCard } from './CacheConnectionCard'

const state = vi.hoisted(() => ({
  required: true,
  mutate: vi.fn(),
}))

vi.mock('../../../queries/databaseAccess', () => ({
  useDatabaseNetwork: () => ({
    data: {
      internal: { host: 'db-cache', port: 6379 },
      tls: { enabled: true, required: state.required },
    },
  }),
  useSetTlsOptional: () => ({ isPending: false, mutate: state.mutate }),
}))
vi.mock('../../databaseAccess/RevealCredentialCard', () => ({
  RevealCredentialCard: () => <div>reveal card</div>,
}))

const database = { name: 'cache', engine: 'redis' } as DatabaseResource

describe('CacheConnectionCard', () => {
  afterEach(() => {
    cleanup()
    state.mutate.mockReset()
  })

  it('shows host, tls port and the reveal card, and switches to optional', () => {
    state.required = true
    render(
      <DatabasesI18n>
        <CacheConnectionCard database={database} />
      </DatabasesI18n>,
    )
    expect(screen.getAllByText(/db-cache/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/6380/).length).toBeGreaterThan(0)
    expect(screen.getByText('reveal card')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Allow plain too' }))
    expect(state.mutate).toHaveBeenCalledTimes(1)
  })

  it('hides the optional switch when TLS is already optional', () => {
    state.required = false
    render(
      <DatabasesI18n>
        <CacheConnectionCard database={database} />
      </DatabasesI18n>,
    )
    expect(screen.queryByRole('button', { name: 'Allow plain too' })).toBeNull()
  })
})
