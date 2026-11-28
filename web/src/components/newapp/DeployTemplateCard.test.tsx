import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { createInstance } from 'i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import newApp from '../../locales/en/newApp.json'
import { parseDeployLink } from '../../lib/deployLink'
import DeployTemplateCard from './DeployTemplateCard'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock('../TemplateLogoTile', () => ({ TemplateLogoTile: () => null }))

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('deploy link with a template', () => {
  it('preselects the template id from ?deploy=1&template=wg-easy', () => {
    const p = parseDeployLink({ deploy: '1', template: 'wg-easy' })
    expect(p.template).toBe('wg-easy')
    expect(p.ignored).toEqual([])
  })

  it('renders platform and grant chips for the template', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              id: 'wg-easy',
              name: 'WG Easy',
              slogan: 'VPN',
              compose: '',
              arch: { amd64: 'tested', arm64: 'absent' },
              requirements: {
                level: 'elevated',
                findings: [],
                protections: [],
                capabilities: [
                  {
                    class: 'net_admin',
                    tier: 'elevated',
                    service: 'wg',
                    reason: 'Creates a tunnel.',
                  },
                ],
              },
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        ),
      ),
    )
    const i18n = createInstance()
    await i18n
      .use(initReactI18next)
      .init({ lng: 'en', defaultNS: 'newApp', resources: { en: { newApp } } })
    render(
      <QueryClientProvider client={new QueryClient()}>
        <I18nextProvider i18n={i18n}>
          <DeployTemplateCard id="wg-easy" />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    expect(await screen.findByText('net_admin')).toBeTruthy()
    expect(screen.getByText('amd64')).toBeTruthy()
    expect(screen.queryByText('arm64')).toBeNull()
  })
})
