import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DnsProviderFields } from './DnsProviderFields'
import integrationsEn from '../../locales/en/integrations.json'
import type { FieldDescriptor } from '../../types/fieldDescriptor'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['integrations'],
  defaultNS: 'integrations',
  resources: { en: { integrations: integrationsEn } },
  interpolation: { escapeValue: false },
})

function field(
  id: string,
  key: string,
  provider: string,
  type: FieldDescriptor['type'],
): FieldDescriptor {
  const p = `integrations:fields.dnsProvider.${id}`
  return {
    id,
    key,
    type,
    label_key: `${p}.label`,
    help_key: `${p}.help`,
    required: true,
    secret: type === 'secret',
    show_when: [{ field: 'provider', equals: [provider] }],
    doc: '',
  }
}

const descriptors = {
  name: 'dns-providers',
  fields: [
    field('cloudflare.token', 'token', 'cloudflare', 'secret'),
    field('route53.region', 'region', 'route53', 'string'),
  ],
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DnsProviderFields', () => {
  it('renders only the chosen provider fields and keeps secrets as password inputs', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify(descriptors), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          }),
        ),
      ),
    )
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <I18nextProvider i18n={testI18n}>
          <DnsProviderFields
            provider="cloudflare"
            values={{ token: '' }}
            onChange={() => undefined}
            idPrefix="t"
            hasSecret
          />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    const input = await screen.findByLabelText('Cloudflare API token')
    expect(input).toHaveAttribute('type', 'password')
    expect(input).toHaveAttribute('placeholder', '•'.repeat(12))
    expect(screen.queryByLabelText('Region (optional)')).toBeNull()
  })
})
