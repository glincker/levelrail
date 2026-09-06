import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CreateDatabaseFields } from './CreateDatabaseFields'

const navigateMock = vi.fn()

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigateMock,
    Link: ({ children, to }: { children?: ReactNode; to?: string }) => (
      <a href={to}>{children}</a>
    ),
  }
})

const createDatabaseMutate = vi.fn()
const setDatabaseNodeMutate = vi.fn()

vi.mock('../queries/databases', () => ({
  useCreateDatabase: () => ({
    mutate: createDatabaseMutate,
    isPending: false,
    isError: false,
    reset: vi.fn(),
  }),
  useSetDatabaseNode: () => ({ mutate: setDatabaseNodeMutate }),
}))

// The registry this file's own variant picker reads: postgres carries a
// non-empty variants list, every other engine doesn't, mirroring
// database_engines.yaml's real shape (only postgres has one today).
vi.mock('../queries/databaseEngines', () => ({
  useDatabaseEnginesOptional: () => ({
    data: [
      {
        id: 'postgres',
        label: 'Postgres',
        default_version: '16',
        variants: [
          { id: 'pgvector', label: 'pgvector' },
          { id: 'postgis', label: 'PostGIS' },
          { id: 'timescaledb', label: 'TimescaleDB' },
        ],
      },
      { id: 'redis', label: 'Redis', default_version: '7' },
    ],
  }),
}))

vi.mock('../queries/nodes', () => ({
  useNodeListOptional: () => ({ data: [] }),
}))

vi.mock('../queries/projects', () => ({
  useProjectListOptional: () => ({ data: [] }),
}))

vi.mock('../queries/systemStatus', () => ({
  useSystemStatusOptional: () => ({ data: { secrets_configured: true } }),
}))

function renderForm(engine?: string) {
  const onCreated = vi.fn()
  render(
    <CreateDatabaseFields open onCreated={onCreated} engine={engine} />,
  )
  return { onCreated }
}

describe('CreateDatabaseFields variant picker', () => {
  beforeEach(() => {
    createDatabaseMutate.mockReset()
    setDatabaseNodeMutate.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows the image variant picker once postgres is the selected engine', async () => {
    const user = userEvent.setup()
    renderForm()

    await screen.findByLabelText('Name')
    // The Engine Select defaults to the registry's first entry
    // (postgres, per defaultValues in CreateDatabaseFields.tsx), so the
    // variant field should already be visible with no interaction.
    expect(screen.getByText('Image variant')).toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: /Engine|postgres/i }))
    const redisOption = await screen.findByRole('option', { name: 'Redis' })
    await user.click(redisOption)

    await waitFor(() => {
      expect(screen.queryByText('Image variant')).not.toBeInTheDocument()
    })
  })

  it('does not render a variant picker for a non-postgres engine fixed by the wizard', async () => {
    renderForm('redis')
    await screen.findByLabelText('Name')
    expect(screen.queryByText('Image variant')).not.toBeInTheDocument()
  })

  it('sends the chosen variant on submit, and omits it when left at default', async () => {
    const user = userEvent.setup()
    renderForm('postgres')

    await screen.findByLabelText('Name')
    await user.type(screen.getByLabelText('Name'), 'vectors')
    await user.type(screen.getByLabelText('Version'), '16')

    await user.click(screen.getByRole('combobox', { name: /Image variant|Default/i }))
    const pgvectorOption = await screen.findByRole('option', { name: 'pgvector' })
    await user.click(pgvectorOption)

    await user.click(screen.getByRole('button', { name: 'Create database' }))

    await waitFor(() => {
      expect(createDatabaseMutate).toHaveBeenCalledTimes(1)
    })
    const [payload] = createDatabaseMutate.mock.calls[0] as [
      { name: string; engine: string; version: string; variant?: string },
    ]
    expect(payload.variant).toBe('pgvector')
  })

  it('omits variant entirely when the default (vanilla) option is kept', async () => {
    const user = userEvent.setup()
    renderForm('postgres')

    await screen.findByLabelText('Name')
    await user.type(screen.getByLabelText('Name'), 'main')
    await user.type(screen.getByLabelText('Version'), '16')

    await user.click(screen.getByRole('button', { name: 'Create database' }))

    await waitFor(() => {
      expect(createDatabaseMutate).toHaveBeenCalledTimes(1)
    })
    const [payload] = createDatabaseMutate.mock.calls[0] as [
      { name: string; engine: string; version: string; variant?: string },
    ]
    expect(payload.variant).toBeUndefined()
  })
})
