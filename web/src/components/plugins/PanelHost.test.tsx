import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/plugins.json'
import { PanelHost } from './PanelHost'
import { safeHref } from '../../lib/safeHref'
import { SafeMarkdown } from './SafeMarkdown'
import { createTestQueryClient } from '@/test/testQueryClient'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['plugins'],
  defaultNS: 'plugins',
  resources: { en: { plugins: en } },
  interpolation: { escapeValue: false },
})

afterEach(() => vi.unstubAllGlobals())

function wrap(node: React.ReactNode) {
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>{node}</I18nextProvider>
    </QueryClientProvider>,
  )
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('SafeMarkdown', () => {
  it('never renders raw HTML or unsafe links', () => {
    const { container } = render(
      <SafeMarkdown
        text={
          '<img src=x onerror=alert(1)> **bold** [bad](javascript:alert(1)) [ok](https://example.com/a)'
        }
      />,
    )
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('strong')?.textContent).toBe('bold')
    const links = container.querySelectorAll('a')
    expect(links).toHaveLength(1)
    expect(links[0]?.getAttribute('rel')).toContain('noopener')
    expect(container.innerHTML).not.toContain('javascript:')
  })

  it('only allows http and https hrefs', () => {
    expect(safeHref('https://example.com')).toBe('https://example.com/')
    expect(safeHref('data:text/html,x')).toBeNull()
    expect(safeHref('//evil.example')).toBeNull()
  })
})

describe('PanelHost', () => {
  const entry = {
    plugin: 'deploy-panel',
    publisher: 'Example Org',
    panel: {
      id: 'deploys',
      title: 'Recent deploys',
      description: '',
      slot: 'app-detail',
      widgets: [],
    },
  }

  it('renders a table from the plugin data call', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) =>
        Promise.resolve(
          url.includes('/data')
            ? json({
                widgets: [
                  {
                    type: 'table',
                    title: 'Latest',
                    columns: [{ key: 'sha', label: 'Commit' }],
                    rows: [['abc123']],
                  },
                ],
              })
            : json([entry]),
        ),
      ),
    )
    wrap(<PanelHost slot="app-detail" appId="web" />)
    expect(await screen.findByText('abc123')).toBeTruthy()
    expect(screen.getByText('Commit')).toBeTruthy()
  })

  it('shows a per-panel error with Retry', async () => {
    let calls = 0
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (!url.includes('/data')) return Promise.resolve(json([entry]))
        calls += 1
        return Promise.resolve(
          calls === 1
            ? json({ error: 'the plugin is not running' }, 503)
            : json({ widgets: [{ type: 'stat', title: 'Total', value: '7' }] }),
        )
      }),
    )
    wrap(<PanelHost slot="app-detail" appId="web" />)
    const retry = await screen.findByRole('button', { name: 'Retry' })
    await userEvent.click(retry)
    await waitFor(() => expect(screen.getByText('7')).toBeTruthy())
  })

  it('renders nothing when no plugin targets the slot', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(json([]))),
    )
    const { container } = wrap(<PanelHost slot="dashboard" />)
    await waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(container.textContent).toBe('')
  })
})
