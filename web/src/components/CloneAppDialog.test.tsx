import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CloneAppDialog } from './CloneAppDialog'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return { ...actual, useNavigate: () => vi.fn() }
})

const preview = {
  source: 'web',
  will_copy: ['image, port'],
  will_not_copy: ['domains', 'volume data'],
  secret_names: ['API_KEY', 'DB_URL'],
  source_domains: ['web.example.com'],
}

const bodies: unknown[] = []

function stubFetch() {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const isPreview = String(url).endsWith('/clone/preview')
      if (!isPreview && init?.body) bodies.push(JSON.parse(init.body as string))
      const body = isPreview ? preview : { name: 'web-2', image: 'nginx' }
      return Promise.resolve({
        ok: true,
        status: isPreview ? 200 : 201,
        json: () => Promise.resolve(body),
      } as unknown as Response)
    }),
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  bodies.length = 0
})

function renderDialog() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <CloneAppDialog name="web" control={{ open: true, hideTrigger: true }} />
    </QueryClientProvider>,
  )
}

describe('CloneAppDialog', () => {
  it('sends no secrets and no domains by default', async () => {
    stubFetch()
    const user = userEvent.setup()
    renderDialog()
    await screen.findByText('Not copied')
    await user.type(screen.getByLabelText('New app name'), 'web-2')
    await user.click(screen.getByRole('button', { name: 'Clone app' }))
    await waitFor(() => {
      expect(bodies).toHaveLength(1)
    })
    expect(bodies[0]).toEqual({ new_name: 'web-2' })
  })

  it('copies secret references and derives domains when opted in', async () => {
    stubFetch()
    const user = userEvent.setup()
    renderDialog()
    await screen.findByText('Not copied')
    await user.click(
      screen.getByRole('checkbox', { name: /Copy 2 secret values/ }),
    )
    await user.click(
      screen.getByRole('checkbox', { name: /Derive new domains/ }),
    )
    await user.type(screen.getByLabelText('New app name'), 'web-2')
    await user.click(screen.getByRole('button', { name: 'Clone app' }))
    await waitFor(() => {
      expect(bodies).toHaveLength(1)
    })
    expect(bodies[0]).toEqual({
      new_name: 'web-2',
      copy_secrets: true,
      domains: 'suffix',
      domain_suffix: 'web-2',
    })
  })
})
