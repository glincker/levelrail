import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { DomainCheckPanel, type DomainCheckPanelData } from './DomainDnsCheck'
import domainsEn from '../locales/en/domains.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

function renderPanel(
  data: DomainCheckPanelData | undefined,
  opts: { isFetching?: boolean; onRefetch?: () => void } = {},
) {
  render(
    <I18nextProvider i18n={testI18n}>
      <DomainCheckPanel
        domain="app.example.com"
        data={data}
        isFetching={opts.isFetching ?? false}
        onRefetch={opts.onRefetch ?? (() => undefined)}
      />
    </I18nextProvider>,
  )
}

describe('DomainCheckPanel', () => {
  it('shows only the domain and a Check now button before any result', () => {
    renderPanel(undefined)
    expect(screen.getByText('app.example.com')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /check now/i })).toBeEnabled()
    expect(screen.queryByText(/Add this record/)).not.toBeInTheDocument()
  })

  it('shows a single record row set for a raw IPv4 host', () => {
    renderPanel({ status: 'not_resolving', expected_host: '203.0.113.7' })
    expect(screen.getByText('Not resolving yet')).toBeInTheDocument()
    expect(screen.getByText(/Add this record/)).toBeInTheDocument()
    expect(screen.getByText('A')).toBeInTheDocument()
    expect(screen.getByText('203.0.113.7')).toBeInTheDocument()
  })

  it('offers CNAME first and an A alternative for a hostname', async () => {
    renderPanel({
      status: 'not_resolving',
      expected_host: 'edge.example.net',
      expected_ipv4: ['198.51.100.4'],
    })
    expect(screen.getByText(/one of these records/)).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'CNAME' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: 'A' }))
    expect(await screen.findByText('198.51.100.4')).toBeInTheDocument()
  })

  it('hides the record block once connected', () => {
    renderPanel({ status: 'connected', expected_host: '203.0.113.7' })
    expect(screen.getByText('Connected')).toBeInTheDocument()
    expect(screen.queryByText(/Add this record/)).not.toBeInTheDocument()
  })

  it('warns when the domain resolves somewhere else', () => {
    renderPanel({
      status: 'resolves_elsewhere',
      expected_host: '203.0.113.7',
    })
    expect(screen.getByText('Points elsewhere')).toBeInTheDocument()
    expect(screen.getByText(/resolves somewhere else/)).toBeInTheDocument()
  })

  it('flags an inferred host as a best guess', () => {
    renderPanel({
      status: 'connected',
      expected_host: '203.0.113.7',
      host_inferred: true,
    })
    expect(screen.getByText(/best guess/)).toBeInTheDocument()
  })

  it('lists each resolver answer or its error', () => {
    renderPanel({
      status: 'propagating',
      expected_host: '203.0.113.7',
      resolvers: [
        { name: 'cloudflare', addresses: ['203.0.113.7'] },
        { name: 'google', error: 'timeout' },
      ],
    })
    expect(screen.getByText('cloudflare')).toBeInTheDocument()
    expect(screen.getByText('timeout')).toBeInTheDocument()
  })

  it('calls onRefetch from Check now and disables it while fetching', async () => {
    const onRefetch = vi.fn()
    renderPanel({ status: 'not_resolving' }, { onRefetch })
    await userEvent.click(screen.getByRole('button', { name: /check now/i }))
    expect(onRefetch).toHaveBeenCalledTimes(1)
  })

  it('disables the button while a check is running', () => {
    renderPanel({ status: 'not_resolving' }, { isFetching: true })
    expect(screen.getByRole('button', { name: /checking/i })).toBeDisabled()
  })
})
