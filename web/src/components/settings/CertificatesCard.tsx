import { Link } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  ShieldCheckIcon,
  WarningCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  CardAction,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useCertificates } from '@/queries/certificates'
import type { CertificateStatus } from '@/queries/certificates'

// certStatusMeta maps internal/api/certificates.go's three Status
// values to a Badge variant and label. No fourth "renewal failed" state:
// this codebase's TLS automation only drives Caddy's offline internal
// issuer today (real ACME is a separate, still-open gap), so expiry is
// the one honest signal the backend actually has, see
// queries/certificates.ts's own CertificateStatus doc comment.
const certStatusMeta: Record<
  CertificateStatus['status'],
  { label: string; variant: 'success' | 'warning' | 'destructive' }
> = {
  healthy: { label: 'Healthy', variant: 'success' },
  expiring_soon: { label: 'Expiring soon', variant: 'warning' },
  expired: { label: 'Expired', variant: 'destructive' },
}

function CertificateRow({ cert }: { cert: CertificateStatus }) {
  // Deliberately no "N days left" countdown here: computing that needs
  // Date.now() at render time, which react-hooks/purity flags as an
  // impure render call (the "now" it captures would silently go stale
  // between renders anyway). The absolute date plus the badge's
  // healthy/expiring_soon/expired state already carries the same
  // information without it.
  const notAfter = new Date(cert.not_after)
  const meta = certStatusMeta[cert.status]
  const StatusIcon =
    cert.status === 'healthy'
      ? CheckCircleIcon
      : cert.status === 'expiring_soon'
        ? WarningCircleIcon
        : XCircleIcon

  return (
    <div className="flex items-center justify-between gap-3 py-2 text-sm">
      <div className="min-w-0">
        <p className="truncate font-medium text-foreground">{cert.domain}</p>
        <p className="text-xs text-muted-foreground">
          {cert.status === 'expired' ? 'Expired' : 'Expires'}{' '}
          {notAfter.toLocaleDateString()}
        </p>
      </div>
      <Badge variant={meta.variant}>
        <StatusIcon aria-hidden="true" />
        {meta.label}
      </Badge>
    </div>
  )
}

// CertificatesCard is a passive summary only: for renewing a certificate
// now or uploading a custom one, this links to settings/certificates.tsx
// (the certificate center), which owns those actions. This project
// treats "a cert renewal fails silently at 3am" as its central risk;
// this card is the glanceable surface that makes an at-risk certificate
// visible before that happens.
export function CertificatesCard() {
  const { data: certificates } = useCertificates()

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <ShieldCheckIcon className="size-4" aria-hidden="true" />
          </div>
          <div>
            <CardTitle>TLS certificates</CardTitle>
            <CardDescription>
              Automatic HTTPS certificate status per domain.
            </CardDescription>
          </div>
        </div>
        <CardAction>
          <Button
            type="button"
            variant="outline"
            size="sm"
            render={<Link to="/settings/certificates" />}
          >
            Manage
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent
        className={certificates.length > 0 ? 'divide-y divide-border' : ''}
      >
        {certificates.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No certificates issued yet.
          </p>
        ) : (
          certificates.map((cert) => (
            <CertificateRow key={cert.domain} cert={cert} />
          ))
        )}
      </CardContent>
    </Card>
  )
}
