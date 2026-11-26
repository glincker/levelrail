import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import dbBranches from '../../../locales/en/dbBranches.json'
import type { BranchResource } from '../../../queries/databaseBranches'
import { BranchesPage, CreateBranchDialog } from './BranchesPage'
import { orderTree } from './branchTree'

const branch = (name: string, parent: string): BranchResource => ({
  name,
  parent,
  root: 'main',
  mode: 'full',
  state: 'ready',
  protected: false,
  size_bytes: 1024,
  created_at: '2026-01-01T00:00:00Z',
})
const list = vi.hoisted(() => ({ branches: [] as BranchResource[] }))

vi.mock('../../../queries/databaseBranches', async (orig) => {
  const actual =
    await orig<typeof import('../../../queries/databaseBranches')>()
  const mutation = { mutate: vi.fn(), error: null, isPending: false }
  return {
    ...actual,
    useDatabaseBranches: () => ({
      data: { root: 'main', branches: list.branches },
      isLoading: false,
      error: null,
    }),
    useBranchCapabilities: () => ({
      data: {
        branches: true,
        modes: { full: { available: true }, schema: { available: true } },
      },
    }),
    useCreateBranch: () => mutation,
    useProtectBranch: () => mutation,
    useBranchAction: () => mutation,
  }
})

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { dbBranches } },
  defaultNS: 'dbBranches',
  interpolation: { escapeValue: false },
})
const wrap = (ui: React.ReactNode) => (
  <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
)

describe('BranchesPage', () => {
  it('orders children under their parent', () => {
    const rows = orderTree([branch('b', 'a'), branch('a', ''), branch('c', '')])
    expect(rows.map((r) => r.branch.name)).toEqual(['a', 'b', 'c'])
    expect(rows[1]?.depth).toBe(1)
  })

  it('shows the empty state with a create button', () => {
    list.branches = []
    render(wrap(<BranchesPage name="main" />))
    expect(screen.getByText(dbBranches.empty)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create branch' })).toBeEnabled()
  })

  it('lists branches and renders the create form fields', () => {
    list.branches = [branch('feature', '')]
    render(
      wrap(
        <>
          <BranchesPage name="main" />
          <CreateBranchDialog
            database="main"
            branches={list.branches}
            open
            onOpenChange={() => undefined}
          />
        </>,
      ),
    )
    expect(screen.getAllByText('feature').length).toBeGreaterThan(0)
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
    expect(screen.getByLabelText('Mode')).toBeInTheDocument()
  })
})
