import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { AuditLogTable } from './AuditLogTable'
import auditLogEn from '../locales/en/auditLog.json'
import type { AuditLogEntry } from '../queries/auditLog'

// A dedicated instance with the real auditLog.json resources loaded
// synchronously, not the app's lazyBackend-driven singleton (src/i18n):
// tests want deterministic, already-loaded translations, proof the real
// JSON content (not a stub) renders through useTranslation, the same
// pattern AutoRollbackCard.test.tsx establishes.
const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['auditLog'],
  defaultNS: 'auditLog',
  resources: { en: { auditLog: auditLogEn } },
  interpolation: { escapeValue: false },
})

function entry(overrides: Partial<AuditLogEntry>): AuditLogEntry {
  return {
    id: 'aud_1',
    actor_type: 'system',
    actor_id: 'ingress',
    actor_name: 'Automatic TLS',
    ability: 'cert.renewed',
    method: 'EVENT',
    path: '/api/v1/certificates/app.example.com',
    status_code: 200,
    remote_addr: '',
    created_at: '2026-10-03T00:00:00.000000000Z',
    client_kind: 'system',
    ...overrides,
  }
}

function renderTable(entries: AuditLogEntry[]) {
  return render(
    <I18nextProvider i18n={testI18n}>
      <AuditLogTable entries={entries} />
    </I18nextProvider>,
  )
}

describe('AuditLogTable', () => {
  it('renders the real translated label for a system-recorded renewal, with its domain', () => {
    renderTable([entry({})])
    expect(screen.getByText('Renewed Certificate')).toBeInTheDocument()
    expect(screen.getByText('app.example.com')).toBeInTheDocument()
  })

  it('renders the real translated label for a system-recorded first issuance', () => {
    renderTable([entry({ ability: 'cert.issued' })])
    expect(screen.getByText('Issued Certificate')).toBeInTheDocument()
  })

  it('falls back to the raw ability string for an entry with no label mapping', () => {
    renderTable([
      entry({ ability: 'write', method: 'PUT', path: '/api/v1/apps/web' }),
    ])
    expect(screen.getByText('write')).toBeInTheDocument()
  })
})
