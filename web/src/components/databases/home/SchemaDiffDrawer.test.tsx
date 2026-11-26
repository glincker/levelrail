import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import dbBranches from '../../../locales/en/dbBranches.json'
import type { SchemaDiff } from '../../../queries/databaseBranches'
import { SchemaDiffDrawer } from './SchemaDiffDrawer'

const state = vi.hoisted(() => ({
  result: {},
}))

vi.mock('../../../queries/databaseBranches', async (orig) => {
  const actual =
    await orig<typeof import('../../../queries/databaseBranches')>()
  return { ...actual, useBranchSchemaDiff: () => state.result }
})

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { dbBranches } },
  defaultNS: 'dbBranches',
  interpolation: { escapeValue: false },
})

const diff = (over: Partial<SchemaDiff> = {}): SchemaDiff => ({
  branch: 'b1',
  against: 'main',
  summary: { added: 1, removed: 1, changed: 0 },
  changes: [
    { object: 'table', kind: 'added', schema: 'public', name: 'orders' },
    {
      object: 'column',
      kind: 'removed',
      schema: 'public',
      table: 'users',
      name: 'legacy',
    },
  ],
  sql: 'CREATE TABLE x ();',
  truncated: false,
  ...over,
})

function open() {
  render(
    <I18nextProvider i18n={i18n}>
      <SchemaDiffDrawer
        database="main"
        branch="b1"
        branches={[]}
        onClose={vi.fn()}
      />
    </I18nextProvider>,
  )
}

describe('SchemaDiffDrawer', () => {
  beforeEach(() => {
    state.result = {}
  })

  it('groups changes and copies the migration SQL', async () => {
    state.result = { data: diff() }
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    open()
    expect(screen.getByText('1 added')).toBeTruthy()
    expect(screen.getByText('public.orders')).toBeTruthy()
    expect(screen.getByText('public.users.legacy')).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'Copy migration SQL' }),
    )
    expect(writeText).toHaveBeenCalledWith('CREATE TABLE x ();')
  })

  it('shows the empty state without a copy button', () => {
    state.result = {
      data: diff({
        changes: [],
        sql: '',
        summary: { added: 0, removed: 0, changed: 0 },
      }),
    }
    open()
    expect(screen.getByText('No schema differences')).toBeTruthy()
    expect(
      screen.queryByRole('button', { name: 'Copy migration SQL' }),
    ).toBeNull()
  })

  it('shows an error', () => {
    state.result = { error: new Error('boom') }
    open()
    expect(screen.getByText(/boom/)).toBeTruthy()
  })
})
