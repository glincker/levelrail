import type { CertificateStatus } from '../queries/certificates'
import type { Domain } from '../queries/domains'

// One row of the certificate center (routes/settings/certificates.tsx):
// a domain joined with its stored certificate, if any. appName is
// undefined only for an orphaned certificate (a domain removed from its
// app since issuance): neither renewal nor custom-cert upload is
// app-scope-able without one, so the page shows that row informationally
// only.
export interface CertificateCenterRow {
  domain: string
  appName?: string
  cert?: CertificateStatus
}

// mergeCertificateCenterRows joins every app-owned domain (GET
// /api/v1/domains) with its stored certificate (GET /api/v1/certificates)
// by exact domain match, the same join key routes/domains/index.tsx's own
// certByDomain map uses. Any certificate matching no configured domain is
// appended as an orphaned row. Sorted by domain so the page reads the
// same on every load.
export function mergeCertificateCenterRows(
  domains: Domain[],
  certificates: CertificateStatus[],
): CertificateCenterRow[] {
  const certByDomain = new Map(certificates.map((c) => [c.domain, c] as const))
  const seen = new Set<string>()
  const rows: CertificateCenterRow[] = []

  for (const d of domains) {
    seen.add(d.domain)
    rows.push({
      domain: d.domain,
      appName: d.service_name,
      cert: certByDomain.get(d.domain),
    })
  }
  for (const c of certificates) {
    if (seen.has(c.domain)) continue
    seen.add(c.domain)
    rows.push({ domain: c.domain, appName: c.apps?.[0], cert: c })
  }

  return rows.sort((a, b) => a.domain.localeCompare(b.domain))
}
