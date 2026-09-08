import { useState } from 'react'
import { CertificateIcon, LockKeyIcon, ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import {
  useClearDomainTLSCert,
  useDomainTLSCert,
  useSetDomainTLSCert,
} from '../queries/domainTlsCert'

// BYO (bring your own) TLS certificate for one already-saved domain: an
// operator-supplied certificate/key pair used by the embedded Caddy
// ingress in place of automatic ACME/internal issuance
// (internal/reconcile/ingress), for domains ACME cannot reach
// (internal-only hosts, externally issued wildcards, a cert already
// provisioned before DNS cuts over). Collapsed by default like
// DomainBasicAuthControl, and expands into a form for the same reason:
// pasting PEM text needs a real form, not an instant toggle like
// DomainMaintenanceControl.
export function DomainTLSCertControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { data: cert, isLoading } = useDomainTLSCert(appName, domain)
  const [open, setOpen] = useState(false)
  const [certPEM, setCertPEM] = useState('')
  const [keyPEM, setKeyPEM] = useState('')
  const [formError, setFormError] = useState<string | null>(null)
  const setCert = useSetDomainTLSCert(appName, domain)
  const clearCert = useClearDomainTLSCert(appName, domain)

  if (isLoading) {
    return (
      <div
        className="flex items-center justify-between gap-2 rounded-md border border-border bg-muted/30 p-3"
        aria-hidden="true"
      >
        <Skeleton className="h-5 w-28 rounded-full" />
        <Skeleton className="h-7 w-32" />
      </div>
    )
  }

  const uploaded = cert?.enabled ?? false
  const pending = setCert.isPending || clearCert.isPending

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setFormError(null)
    if (!certPEM.trim() || !keyPEM.trim()) {
      setFormError('Both a certificate and a private key are required.')
      return
    }
    setCert.mutate(
      { cert: certPEM.trim(), key: keyPEM.trim() },
      {
        onSuccess: () => {
          setCertPEM('')
          setKeyPEM('')
        },
      },
    )
  }

  return (
    <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          className="flex items-center gap-1.5 text-left"
        >
          {uploaded ? (
            <Badge variant="success" className="shrink-0">
              <ShieldCheckIcon className="size-3" />
              BYO certificate
            </Badge>
          ) : (
            <Badge variant="muted" className="shrink-0">
              <LockKeyIcon className="size-3" />
              Automatic TLS
            </Badge>
          )}
          {uploaded && cert?.expires_at ? (
            <span className="text-xs text-muted-foreground">
              expires {new Date(cert.expires_at).toLocaleDateString()}
            </span>
          ) : null}
        </button>
        <Button type="button" variant="ghost" size="sm" onClick={() => setOpen((v) => !v)}>
          <CertificateIcon className="size-3.5" />
          {open ? 'Hide' : uploaded ? 'Manage' : 'Upload certificate'}
        </Button>
      </div>

      {open ? (
        <form
          onSubmit={(e) => {
            handleSubmit(e)
          }}
          className="mt-3 space-y-3"
        >
          <Field>
            <FieldLabel htmlFor={`tls-cert-pem-${domain}`}>Certificate (PEM)</FieldLabel>
            <Textarea
              id={`tls-cert-pem-${domain}`}
              value={certPEM}
              onChange={(e) => setCertPEM(e.target.value)}
              disabled={pending}
              placeholder="-----BEGIN CERTIFICATE-----"
              className="font-mono text-xs"
              rows={4}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`tls-key-pem-${domain}`}>Private key (PEM)</FieldLabel>
            <Textarea
              id={`tls-key-pem-${domain}`}
              value={keyPEM}
              onChange={(e) => setKeyPEM(e.target.value)}
              disabled={pending}
              placeholder="-----BEGIN PRIVATE KEY-----"
              className="font-mono text-xs"
              rows={4}
            />
            <FieldDescription>
              {uploaded
                ? 'Replaces the currently uploaded certificate entirely; there is no partial update.'
                : 'Used instead of automatic ACME/internal issuance for this domain, starting on the next reconcile pass.'}
            </FieldDescription>
          </Field>

          {formError ? (
            <Alert variant="destructive">
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          ) : null}
          {setCert.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{setCert.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {clearCert.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{clearCert.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={pending}>
              {setCert.isPending ? 'Uploading...' : 'Upload'}
            </Button>
            {uploaded ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={pending}
                onClick={() => {
                  clearCert.mutate()
                }}
              >
                {clearCert.isPending ? 'Reverting...' : 'Revert to automatic TLS'}
              </Button>
            ) : null}
          </div>
        </form>
      ) : null}
    </div>
  )
}
