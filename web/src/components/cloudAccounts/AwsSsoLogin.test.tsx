import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AwsSsoLogin } from './AwsSsoLogin'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

function reply(body: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

async function seen(
  input: string | URL | Request,
  init?: RequestInit,
): Promise<{ url: string; body: string }> {
  if (input instanceof Request) {
    return { url: input.url, body: await input.clone().text() }
  }
  const body = typeof init?.body === 'string' ? init.body : ''
  return { url: typeof input === 'string' ? input : input.href, body }
}

describe('AwsSsoLogin', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows the device code and keeps tokens out of the page', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation((input) => {
      const url = String(input instanceof Request ? input.url : input)
      if (url.endsWith('/aws/sso/start')) {
        return reply({
          session_id: 's1',
          user_code: 'ABCD-EFGH',
          verification_uri: 'https://v',
          verification_uri_complete: 'https://v?c=1',
          interval: 60,
          expires_in: 600,
        })
      }
      return reply({ status: 'pending', interval: 60 })
    })
    const qc = new QueryClient()
    render(
      <QueryClientProvider client={qc}>
        <AwsSsoLogin />
      </QueryClientProvider>,
    )
    const start = screen.getByRole('button', { name: 'sso.start' })
    expect(start).toBeDisabled()
    await userEvent.type(
      screen.getByPlaceholderText('sso.regionExample'),
      'eu-west-1',
    )
    await userEvent.type(
      screen.getByPlaceholderText('sso.startUrlExample'),
      'https://d-1.awsapps.com/start',
    )
    await userEvent.click(start)
    await waitFor(() => {
      expect(screen.getByText('ABCD-EFGH')).toBeInTheDocument()
    })
    expect(
      screen.getByRole('link', { name: 'sso.openBrowser' }),
    ).toHaveAttribute('href', 'https://v?c=1')
  })

  it('sends the optional region with the chosen role', async () => {
    const bodies: Record<string, string> = {}
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const { url, body } = await seen(input, init)
      bodies[url.split('/aws/sso/')[1] ?? url] = body
      if (url.endsWith('/aws/sso/start')) {
        return await reply({
          session_id: 's1',
          user_code: 'ABCD-EFGH',
          verification_uri: 'https://v',
          verification_uri_complete: 'https://v?c=1',
          interval: 0,
          expires_in: 600,
        })
      }
      if (url.endsWith('/aws/sso/poll')) {
        return await reply({
          status: 'done',
          accounts: [
            {
              account_id: '111122223333',
              account_name: 'prod',
              roles: ['Admin'],
            },
          ],
        })
      }
      return await reply({ id: 'aws-1' })
    })
    render(
      <QueryClientProvider client={new QueryClient()}>
        <AwsSsoLogin />
      </QueryClientProvider>,
    )
    await userEvent.type(
      screen.getByPlaceholderText('sso.regionExample'),
      'eu-west-1',
    )
    await userEvent.type(
      screen.getByPlaceholderText('sso.startUrlExample'),
      'https://d-1.awsapps.com/start',
    )
    await userEvent.click(screen.getByRole('button', { name: 'sso.start' }))
    const link = await screen.findByRole('button', { name: 'sso.link' })
    expect(link).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Admin' }))
    await userEvent.type(
      screen.getByPlaceholderText('sso.regionExample'),
      'us-east-1',
    )
    await userEvent.click(link)
    await waitFor(() => {
      expect(bodies.select).toBeDefined()
    })
    const sent = JSON.parse(bodies.select) as Record<string, string>
    expect(sent).toMatchObject({
      session_id: 's1',
      account_id: '111122223333',
      role_name: 'Admin',
      region: 'us-east-1',
    })
  })

  it('signs in again for an existing account without asking for a start url', async () => {
    const onDone = vi.fn()
    const bodies: Record<string, string> = {}
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const { url, body } = await seen(input, init)
      bodies[url.split('/aws/sso/')[1] ?? url] = body
      if (url.endsWith('/aws/sso/relogin')) {
        return await reply({
          session_id: 's2',
          user_code: 'WXYZ-1234',
          verification_uri: 'https://v',
          verification_uri_complete: 'https://v?c=2',
          interval: 0,
          expires_in: 600,
        })
      }
      return await reply({ status: 'done', account_id: 'aws-1' })
    })
    render(
      <QueryClientProvider client={new QueryClient()}>
        <AwsSsoLogin reloginAccountId="aws-1" onDone={onDone} />
      </QueryClientProvider>,
    )
    expect(screen.queryByPlaceholderText('sso.startUrlExample')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'sso.start' }))
    await waitFor(() => {
      expect(screen.getByText('sso.reloginDone')).toBeInTheDocument()
    })
    expect(JSON.parse(bodies.relogin)).toEqual({ account_id: 'aws-1' })
    expect(onDone).toHaveBeenCalled()
  })
})
