import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ImportFrontDoor } from './ImportFrontDoor'
import { ImportPlanPreview } from './ImportPlanPreview'
import type { ImportPlan, ImportService } from '../queries/imports'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return { ...actual, useNavigate: () => navigate }
})

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  } as unknown as Response
}

function withClient(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return <QueryClientProvider client={client}>{ui}</QueryClientProvider>
}

const dockerRunService: ImportService = {
  name: 'pg',
  image: 'postgres:16',
  build: 'image',
  build_reason: 'image from the docker run command',
  port: 5432,
  env: [
    {
      key: 'POSTGRES_PASSWORD',
      required: true,
      has_default: false,
      secret: true,
    },
    {
      key: 'TZ',
      value: 'UTC',
      required: false,
      has_default: true,
      secret: false,
    },
  ],
  volumes: [
    { host_path: '/srv/data', container_path: '/data', needs_approval: true },
  ],
}

const dockerRunPlan: ImportPlan = {
  source: 'docker_run',
  suggested_name: 'pg',
  deploy: 'app',
  warnings: [
    {
      code: 'unsupported_flag',
      message: '--privileged: privileged containers are not supported',
    },
  ],
  missing_required_env: ['POSTGRES_PASSWORD'],
  services: [dockerRunService],
}

afterEach(() => {
  vi.restoreAllMocks()
  navigate.mockReset()
})

describe('ImportFrontDoor', () => {
  it('detects a pasted docker run command and posts it for a plan', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(jsonResponse(dockerRunPlan))
    const onPlan = vi.fn()
    const user = userEvent.setup()
    render(withClient(<ImportFrontDoor onPlan={onPlan} />))

    await user.click(screen.getByRole('textbox'))
    await user.paste('docker run -d -p 5432:5432 postgres:16')
    expect(screen.getByTestId('import-guess')).toHaveTextContent(
      'docker run command',
    )
    await user.click(screen.getByRole('button', { name: 'Preview plan' }))

    await waitFor(() => {
      expect(onPlan).toHaveBeenCalled()
    })
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/imports/plan')
    expect(JSON.parse(init.body as string)).toEqual({
      text: 'docker run -d -p 5432:5432 postgres:16',
    })
  })

  it('shows the server error and keeps the input', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      jsonResponse(
        { error: 'importplan: could not tell what this input is' },
        400,
      ),
    )
    const user = userEvent.setup()
    render(withClient(<ImportFrontDoor onPlan={vi.fn()} />))
    await user.type(screen.getByRole('textbox'), 'hello there')
    await user.click(screen.getByRole('button', { name: 'Preview plan' }))
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByRole('textbox')).toHaveValue('hello there')
  })

  it('fills an example', async () => {
    const user = userEvent.setup()
    render(withClient(<ImportFrontDoor onPlan={vi.fn()} />))
    await user.click(screen.getByRole('button', { name: 'GitHub repo' }))
    expect(screen.getByRole('textbox')).toHaveValue(
      'https://github.com/owner/repo',
    )
    expect(screen.getByTestId('import-guess')).toHaveTextContent(
      'Git repository',
    )
  })
})

describe('ImportPlanPreview', () => {
  it('lists warnings, blocks deploy until required env is filled, then creates the app', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(
        jsonResponse({ name: 'pg', image: 'postgres:16' }, 201),
      )
    const onDeployed = vi.fn()
    const user = userEvent.setup()
    render(
      withClient(
        <ImportPlanPreview
          plan={dockerRunPlan}
          request={{ text: 'docker run postgres:16' }}
          onBack={vi.fn()}
          onDeployed={onDeployed}
        />,
      ),
    )

    expect(
      screen.getByText(/privileged containers are not supported/),
    ).toBeInTheDocument()
    expect(screen.getByText('Needs root approval')).toBeInTheDocument()
    expect(
      screen.getByText(/Required before deploying: POSTGRES_PASSWORD/),
    ).toBeInTheDocument()
    const deployButton = screen.getByRole('button', { name: 'Deploy' })
    expect(deployButton).toBeDisabled()

    await user.type(
      screen.getByLabelText('Value for POSTGRES_PASSWORD'),
      'hunter2',
    )
    expect(deployButton).toBeEnabled()
    await user.click(deployButton)

    await waitFor(() => {
      expect(onDeployed).toHaveBeenCalled()
    })
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/apps')
    const body = JSON.parse(init.body as string) as Record<string, unknown>
    expect(body).toMatchObject({
      name: 'pg',
      image: 'postgres:16',
      port: 5432,
      env: { TZ: 'UTC' },
      secret_env: ['POSTGRES_PASSWORD'],
      secrets: { POSTGRES_PASSWORD: 'hunter2' },
    })
    expect(navigate).toHaveBeenCalledWith({
      to: '/apps/$name',
      params: { name: 'pg' },
    })
  })

  it('keeps the port editable and rejects an empty one', async () => {
    const user = userEvent.setup()
    render(
      withClient(
        <ImportPlanPreview
          plan={{
            ...dockerRunPlan,
            missing_required_env: [],
            services: [{ ...dockerRunService, env: [] }],
          }}
          request={{ text: 'x' }}
          onBack={vi.fn()}
          onDeployed={vi.fn()}
        />,
      ),
    )
    const port = screen.getByLabelText('Container port')
    await user.clear(port)
    expect(screen.getByRole('button', { name: 'Deploy' })).toBeDisabled()
    await user.type(port, '8080')
    expect(screen.getByRole('button', { name: 'Deploy' })).toBeEnabled()
  })

  it('disables deploy for a plan that cannot be deployed alone', () => {
    render(
      withClient(
        <ImportPlanPreview
          plan={{
            ...dockerRunPlan,
            deploy: 'none',
            missing_required_env: [],
            services: [],
          }}
          request={{ text: 'FROM x' }}
          onBack={vi.fn()}
          onDeployed={vi.fn()}
        />,
      ),
    )
    expect(screen.getByRole('button', { name: 'Deploy' })).toBeDisabled()
    expect(screen.getByText(/Nothing to deploy was found/)).toBeInTheDocument()
  })
})
