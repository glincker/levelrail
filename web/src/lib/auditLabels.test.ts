import { describe, expect, it } from 'vitest'
import { auditFriendlyLabel } from './auditLabels'

describe('auditFriendlyLabel', () => {
  it('labels a system-recorded first issuance, with its domain', () => {
    const label = auditFriendlyLabel({
      ability: 'cert.issued',
      method: 'EVENT',
      path: '/api/v1/certificates/prometheus.example.com',
    })
    expect(label).not.toBeNull()
    expect(label?.label).toBe('Issued Certificate')
    expect(label?.domain).toBe('prometheus.example.com')
  })

  it('labels a system-recorded renewal, with its domain', () => {
    const label = auditFriendlyLabel({
      ability: 'cert.renewed',
      method: 'EVENT',
      path: '/api/v1/certificates/prometheus.example.com',
    })
    expect(label?.label).toBe('Renewed Certificate')
    expect(label?.domain).toBe('prometheus.example.com')
  })

  it('labels a manual force-renew request, extracting the domain from the app-scoped path', () => {
    const label = auditFriendlyLabel({
      ability: 'root',
      method: 'POST',
      path: '/api/v1/apps/web/domains/app.example.com/cert/renew',
    })
    expect(label?.label).toBe('Requested Certificate Renewal')
    expect(label?.domain).toBe('app.example.com')
  })

  it('labels a BYO certificate upload', () => {
    const label = auditFriendlyLabel({
      ability: 'root',
      method: 'PUT',
      path: '/api/v1/apps/web/domains/app.example.com/tls-cert',
    })
    expect(label?.label).toBe('Uploaded Certificate')
    expect(label?.domain).toBe('app.example.com')
  })

  it('labels a BYO certificate removal', () => {
    const label = auditFriendlyLabel({
      ability: 'root',
      method: 'DELETE',
      path: '/api/v1/apps/web/domains/app.example.com/tls-cert',
    })
    expect(label?.label).toBe('Removed Certificate')
    expect(label?.domain).toBe('app.example.com')
  })

  it('returns null for an entry with no mapping, so the caller falls back to raw columns', () => {
    expect(
      auditFriendlyLabel({
        ability: 'write',
        method: 'PUT',
        path: '/api/v1/apps/web',
      }),
    ).toBeNull()
  })

  it('decodes a percent-encoded domain segment', () => {
    const label = auditFriendlyLabel({
      ability: 'cert.renewed',
      method: 'EVENT',
      path: '/api/v1/certificates/xn--caf-dma.example.com',
    })
    expect(label?.domain).toBe('xn--caf-dma.example.com')
  })
})
