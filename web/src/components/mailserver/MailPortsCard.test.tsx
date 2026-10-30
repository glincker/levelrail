import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/mailServer.json'
import { MailPortsCard } from './MailPortsCard'

vi.mock('../env/StepUpDialog', () => ({ StepUpDialog: () => null }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { mailServer: en } },
  interpolation: { escapeValue: false },
})

const row = (port: number, bind: string) => ({
  port,
  purpose: 'p',
  published: true,
  bind,
  firewall: 'not_filtering',
})

function portsBody(bind: string) {
  return {
    app: 'mail',
    installed: true,
    ports: [25, 465, 587, 993].map((p) => row(p, bind)),
    web_listener: 'private',
    port_25_note: 'Port 25 provider note',
    warning: 'Public internet warning',
    next_step: '',
    bind_scope_note: '',
  }
}

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <MailPortsCard />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('MailPortsCard', () => {
  it('opens the four ports after the confirm dialog', async () => {
    const calls: { url: string; body?: string }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({ url, body: init?.body as string | undefined })
        const json =
          url === '/api/v1/mail/ports/open'
            ? {
                app: 'mail',
                changed: [25, 465, 587, 993],
                result: 'opened',
                warning: '',
                message: 'Ports opened',
              }
            : portsBody('private')
        return Promise.resolve(
          new Response(JSON.stringify(json), { status: 200 }),
        )
      }),
    )
    renderCard()
    const button = await screen.findByRole('button', { name: en.ports.open })
    expect(screen.getAllByText(en.ports.bind.private)).toHaveLength(4)
    await userEvent.click(button)
    expect(await screen.findByText('Port 25 provider note')).toBeInTheDocument()
    expect(screen.getByText(en.ports.certificate)).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: en.ports.confirm }),
    )
    expect(await screen.findByText('Ports opened')).toBeInTheDocument()
    const post = calls.find((c) => c.url === '/api/v1/mail/ports/open')
    expect(JSON.parse(post?.body ?? '{}')).toEqual({
      app: 'mail',
      confirm: true,
    })
  })

  it('renders nothing without a mail app', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ installed: false, ports: [] }), {
            status: 200,
          }),
        ),
      ),
    )
    const { container } = renderCard()
    await new Promise((r) => setTimeout(r, 20))
    expect(container).toBeEmptyDOMElement()
  })
})
