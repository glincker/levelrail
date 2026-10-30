import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/dnsSetup.json'
import type { MailPlan } from '@/queries/mailDns'
import { DnsRecordTable } from './DnsRecordTable'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { dnsSetup: en } },
  defaultNS: 'dnsSetup',
  interpolation: { escapeValue: false },
})

const longKey = 'v=DKIM1; k=rsa; p=' + 'A'.repeat(300)
const plan: MailPlan = {
  domain: 'example.com',
  hash: 'h',
  summary: {},
  blocked: false,
  changes: [
    {
      action: 'add',
      kind: 'MX',
      set: {
        name: 'example.com',
        type: 'MX',
        ttl: 300,
        values: ['10 mail.example.com'],
      },
    },
    {
      action: 'add',
      kind: 'DKIM',
      set: {
        name: 's._domainkey.example.com',
        type: 'TXT',
        ttl: 300,
        values: [longKey],
      },
    },
    {
      action: 'refused',
      kind: 'SPF',
      set: {
        name: 'example.com',
        type: 'TXT',
        ttl: 300,
        values: ['v=spf1 -all'],
      },
      before: { name: 'example.com', type: 'TXT', ttl: 300, values: ['old'] },
    },
    {
      action: 'refused',
      kind: 'HOST',
      set: {
        name: 'mail.example.com',
        type: 'A',
        ttl: 300,
        values: ['203.0.113.7'],
      },
      reason: 'CNAME in the way',
    },
  ],
}

function renderTable(onToggle?: (k: string) => void) {
  return render(
    <I18nextProvider i18n={i18n}>
      <DnsRecordTable
        kind="mail"
        plan={plan}
        phase="review"
        replaceKinds={[]}
        onToggleReplace={onToggle}
      />
    </I18nextProvider>,
  )
}
afterEach(() => vi.unstubAllGlobals())

describe('DnsRecordTable', () => {
  it('renders one table with group headers and a status chip per row', () => {
    renderTable()
    expect(screen.getAllByRole('table')).toHaveLength(1)
    expect(screen.getByText('Receive mail')).toBeTruthy()
    expect(screen.getByText('Trust')).toBeTruthy()
    const states = screen.getAllByTestId('row-status').map((n) => n.textContent)
    expect(states).toEqual(['Will add', 'Conflict', 'Will add', 'Different'])
    expect(screen.getByText('Found: old')).toBeTruthy()
    expect(screen.getByText('CNAME in the way')).toBeTruthy()
  })

  it('copies a value on click and shows Copied in place', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    renderTable()
    await userEvent.click(
      screen.getAllByRole('button', { name: 'Copy value of example.com' })[0]!,
    )
    expect(writeText).toHaveBeenCalledWith('mail.example.com')
    expect(await screen.findByText('Copied')).toBeTruthy()
  })

  it('truncates long values and expands the full text on demand', async () => {
    renderTable()
    const valueBtn = screen.getByTitle(longKey)
    expect(valueBtn.className).toContain('truncate')
    await userEvent.click(
      screen.getByRole('button', { name: 'Show full value' }),
    )
    expect(document.querySelector('code')?.textContent).toBe(longKey)
    expect(document.querySelector('code')?.className).toContain('break-all')
  })

  it('uses a two-line grid on mobile and hides TTL behind sm', () => {
    renderTable()
    const row = document.querySelector('[data-row] > div')
    expect(row?.className).toContain('grid-cols-[auto_auto_minmax(0,1fr)_auto]')
    expect(row?.className).toContain('sm:grid-cols-')
    expect(document.querySelector('.sm\\:hidden')?.textContent).toContain(
      'TTL 300',
    )
  })

  it('stages Replace for a Different row and moves focus with arrow keys', async () => {
    const onToggle = vi.fn()
    renderTable(onToggle)
    await userEvent.click(screen.getByRole('button', { name: 'Replace' }))
    expect(onToggle).toHaveBeenCalledWith('SPF')
    const rows = document.querySelectorAll<HTMLElement>('[data-row]')
    rows[0]?.focus()
    await userEvent.keyboard('{ArrowDown}')
    expect(document.activeElement).toBe(rows[1])
  })
})
