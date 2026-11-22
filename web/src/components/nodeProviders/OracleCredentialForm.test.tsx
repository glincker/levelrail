import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import nodesEn from '../../locales/en/nodes.json'
import { OracleCredentialForm } from './OracleCredentialForm'
import { OracleGuardNotice } from '../addNode/OracleGuardNotice'
import type { NodeProviderResource } from '../../types/nodeProvision'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

const PEM = '-----BEGIN PRIVATE KEY-----\nsecretbody\n-----END PRIVATE KEY-----'

function renderForm(
  provider: NodeProviderResource,
  onSaved = vi.fn(),
  cloudAccount = false,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <OracleCredentialForm
          provider={provider}
          label="Oracle Cloud"
          onSaved={onSaved}
          requireSubnet={!cloudAccount}
          cloudAccount={cloudAccount}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('OracleCredentialForm', () => {
  it('posts the key then clears it, and never shows a stored key', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ provider: 'oracle', has_token: true }), {
        status: 200,
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const onSaved = vi.fn()
    renderForm({ provider: 'oracle', has_token: false }, onSaved)

    const fill = (label: string, value: string) =>
      fireEvent.change(screen.getByLabelText(label), { target: { value } })
    fill(
      'Paste your OCI config',
      '[DEFAULT]\nuser=ocid1.user.oc1..bbb\nfingerprint=aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99\ntenancy=ocid1.tenancy.oc1..aaa\nregion=eu-frankfurt-1\nkey_file=~/k.pem\n',
    )
    // Parsed values collapse into a summary instead of six inputs.
    expect(screen.getByRole('status')).toHaveTextContent(
      'Detected: tenancy ocid1...1..aaa, region eu-frankfurt-1',
    )
    fill('Private key (PEM)', PEM)
    fireEvent.click(screen.getByRole('button', { name: /Advanced/ }))
    fill('Subnet OCID', 'ocid1.subnet.oc1..ccc')

    // Compartment follows the tenancy until the user edits it.
    expect(screen.getByLabelText('Compartment OCID')).toHaveValue(
      'ocid1.tenancy.oc1..aaa',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Test and connect' }))

    await vi.waitFor(() => expect(onSaved).toHaveBeenCalled())
    const init = fetchMock.mock.calls[0]?.[1] as { body: string }
    const body: unknown = JSON.parse(init.body)
    expect(body).toMatchObject({
      method: 'api_key',
      private_key_pem: PEM,
      compartment_ocid: 'ocid1.tenancy.oc1..aaa',
    })
    expect(screen.getByLabelText('Private key (PEM)')).toHaveValue('')
  })

  it('creates a cloud account without a subnet and refreshes the list', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ id: 'a1' })))
    vi.stubGlobal('fetch', fetchMock)
    const onSaved = vi.fn()
    renderForm({ provider: 'oracle', has_token: false }, onSaved, true)
    fireEvent.change(screen.getByLabelText('Paste your OCI config'), {
      target: {
        value:
          '[DEFAULT]\nuser=ocid1.user.oc1..bbb\nfingerprint=aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99\ntenancy=ocid1.tenancy.oc1..aaa\nregion=eu-frankfurt-1\n',
      },
    })
    fireEvent.change(screen.getByLabelText('Private key (PEM)'), {
      target: { value: PEM },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Test and connect' }))

    await vi.waitFor(() => expect(onSaved).toHaveBeenCalled())
    const [url, init] = fetchMock.mock.calls[0] as [string, { body: string }]
    expect(url).toBe('/api/v1/cloud-accounts')
    const body = JSON.parse(init.body) as Record<string, unknown>
    expect(body).toMatchObject({ provider: 'oracle', method: 'api_key' })
    expect(body).not.toHaveProperty('subnet_ocid')
  })

  it('shows a placeholder, not the key, when one is stored', () => {
    renderForm({ provider: 'oracle', has_token: true })
    const key = screen.getByLabelText('Private key (PEM)')
    expect(key).toHaveValue('')
    expect(key).toHaveAttribute(
      'placeholder',
      'A key is stored. Paste a new one to replace it.',
    )
    expect(document.body.textContent).not.toContain('BEGIN PRIVATE KEY')
  })
})

describe('OracleGuardNotice', () => {
  it('names the setting behind a refusal', () => {
    render(
      <I18nextProvider i18n={testI18n}>
        <OracleGuardNotice message="Oracle provisioning is off, turn on provision.oracle.enabled first" />
      </I18nextProvider>,
    )
    expect(
      screen.getByText(/Turn on the setting provision\.oracle\.enabled first/),
    ).toBeInTheDocument()
  })

  it('points at the A1 limits when the allowance is exceeded', () => {
    render(
      <I18nextProvider i18n={testI18n}>
        <OracleGuardNotice message="Oracle A1 OCPU limit: 4 existing plus 1 requested is above provision.oracle.max_a1_ocpu (4)" />
      </I18nextProvider>,
    )
    expect(
      screen.getByText(/provision\.oracle\.max_a1_memory_gb/),
    ).toBeInTheDocument()
  })
})
