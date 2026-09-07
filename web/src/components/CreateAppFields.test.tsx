import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CreateAppFields } from './CreateAppFields'

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

const createAppMutate = vi.fn(
  (
    _req: unknown,
    opts?: { onSuccess?: (created: { name: string }) => void },
  ) => {
    opts?.onSuccess?.({ name: 'myapp' })
  },
)
const setAppNodeMutate = vi.fn()
const createDatabaseMutate = vi.fn(
  (_req: unknown, opts?: { onSuccess?: () => void }) => {
    opts?.onSuccess?.()
  },
)
const setAppDatabaseMutate = vi.fn()

vi.mock('../queries/apps', () => ({
  useCreateApp: () => ({
    mutate: createAppMutate,
    isPending: false,
    isError: false,
    reset: vi.fn(),
  }),
  useSetAppNode: () => ({ mutate: setAppNodeMutate }),
  useSetAppDatabase: () => ({ mutate: setAppDatabaseMutate }),
}))

vi.mock('../queries/databases', () => ({
  useCreateDatabase: () => ({ mutate: createDatabaseMutate }),
}))

vi.mock('../queries/databaseEngines', () => ({
  useDatabaseEnginesOptional: () => ({
    data: [
      { id: 'postgres', label: 'Postgres', default_version: '16' },
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

function renderForm() {
  const onCreated = vi.fn()
  render(<CreateAppFields open onCreated={onCreated} />)
  return { onCreated }
}

async function fillRequiredFields(
  user: ReturnType<typeof userEvent.setup>,
) {
  await user.type(screen.getByLabelText('Name'), 'myapp')
  await user.type(screen.getByLabelText('Image'), 'ghcr.io/org/myapp:latest')
  await user.type(screen.getByLabelText('Port'), '3000')
}

describe('CreateAppFields database attachment', () => {
  beforeEach(() => {
    createAppMutate.mockClear()
    setAppNodeMutate.mockClear()
    createDatabaseMutate.mockClear()
    setAppDatabaseMutate.mockClear()
    window.localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('hides the environment variable field until an engine is picked', async () => {
    renderForm()
    await screen.findByLabelText('Name')
    expect(screen.queryByLabelText('Environment variable')).not.toBeInTheDocument()
  })

  it('never creates or attaches a database when left at None', async () => {
    const user = userEvent.setup()
    renderForm()

    await fillRequiredFields(user)
    await user.click(screen.getByRole('button', { name: 'Create app' }))

    await waitFor(() => {
      expect(createAppMutate).toHaveBeenCalledTimes(1)
    })
    expect(createDatabaseMutate).not.toHaveBeenCalled()
    expect(setAppDatabaseMutate).not.toHaveBeenCalled()
  })

  it('creates a database named after the app and attaches it when an engine is picked', async () => {
    const user = userEvent.setup()
    renderForm()

    await fillRequiredFields(user)
    await user.click(screen.getByRole('combobox', { name: /Attach a database|None/i }))
    const postgresOption = await screen.findByRole('option', { name: 'Postgres' })
    await user.click(postgresOption)

    expect(screen.getByLabelText('Environment variable')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Create app' }))

    await waitFor(() => {
      expect(createDatabaseMutate).toHaveBeenCalledTimes(1)
    })
    const [dbPayload] = createDatabaseMutate.mock.calls[0] as [
      { name: string; engine: string; version: string },
    ]
    expect(dbPayload).toEqual({ name: 'myapp-db', engine: 'postgres', version: '16' })

    await waitFor(() => {
      expect(setAppDatabaseMutate).toHaveBeenCalledTimes(1)
    })
    const [attachPayload] = setAppDatabaseMutate.mock.calls[0] as [
      { name: string; databaseName: string; envVar: string },
    ]
    expect(attachPayload).toEqual({
      name: 'myapp',
      databaseName: 'myapp-db',
      envVar: 'DATABASE_URL',
    })
  })

  it('sends a custom environment variable name when changed', async () => {
    const user = userEvent.setup()
    renderForm()

    await fillRequiredFields(user)
    await user.click(screen.getByRole('combobox', { name: /Attach a database|None/i }))
    const postgresOption = await screen.findByRole('option', { name: 'Postgres' })
    await user.click(postgresOption)

    const envVarInput = screen.getByLabelText('Environment variable')
    await user.clear(envVarInput)
    await user.type(envVarInput, 'PG_URL')

    await user.click(screen.getByRole('button', { name: 'Create app' }))

    await waitFor(() => {
      expect(setAppDatabaseMutate).toHaveBeenCalledTimes(1)
    })
    const [attachPayload] = setAppDatabaseMutate.mock.calls[0] as [
      { envVar: string },
    ]
    expect(attachPayload.envVar).toBe('PG_URL')
  })
})
