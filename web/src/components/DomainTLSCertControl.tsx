import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CaretRightIcon,
  ShieldCheckIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import {
  useClearDomainTLSCert,
  useDomainTLSCert,
  useSetDomainTLSCert,
} from '../queries/domainTlsCert'

// BYO certificate for one saved domain, used by the embedded ingress instead
// of automatic issuance. Collapsed behind a disclosure: most operators never
// need it, and pasting PEM text needs a real form, not a toggle.
export function DomainTLSCertControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const { data: cert, isLoading } = useDomainTLSCert(appName, domain)
  const [open, setOpen] = useState(false)
  const [certPEM, setCertPEM] = useState('')
  const [keyPEM, setKeyPEM] = useState('')
  const [formError, setFormError] = useState<string | null>(null)
  const [confirmingRevert, setConfirmingRevert] = useState(false)
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
      setFormError(t('proxySetup.byo.bothRequired'))
      return
    }
    setCert.mutate(
      { cert: certPEM.trim(), key: keyPEM.trim() },
      {
        onSuccess: () => {
          setCertPEM('')
          setKeyPEM('')
          toast.add({
            title: t('proxySetup.byo.uploadedToast', { domain }),
            type: 'success',
          })
        },
        onError: (error) => {
          // The backend's own validation reason is the actionable part.
          toast.add({
            title: t('proxySetup.byo.uploadFailed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex w-full items-center gap-1.5 text-left"
      >
        <CaretRightIcon
          className={
            open
              ? 'size-3.5 shrink-0 rotate-90 transition-transform'
              : 'size-3.5 shrink-0 transition-transform'
          }
          aria-hidden="true"
        />
        <span className="font-medium text-foreground">
          {t('proxySetup.byo.disclosure')}
        </span>
        {uploaded ? (
          <Badge variant="success" className="shrink-0">
            <ShieldCheckIcon className="size-3" aria-hidden="true" />
            {t('proxySetup.byo.uploaded')}
          </Badge>
        ) : null}
        {uploaded && cert?.expires_at ? (
          <span className="text-xs text-muted-foreground">
            {t('proxySetup.byo.expires', {
              date: new Date(cert.expires_at).toLocaleDateString(),
            })}
          </span>
        ) : null}
      </button>
      {open ? null : (
        <p className="mt-1 pl-5 text-xs text-muted-foreground">
          {t('proxySetup.byo.hint')}
        </p>
      )}

      {open ? (
        <form
          onSubmit={(e) => {
            handleSubmit(e)
          }}
          className="mt-3 space-y-3"
        >
          <p className="text-xs text-muted-foreground">
            {t('proxySetup.byo.hint')}
          </p>
          <Field>
            <FieldLabel htmlFor={`tls-cert-pem-${domain}`}>
              {t('proxySetup.byo.certLabel')}
            </FieldLabel>
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
            <FieldLabel htmlFor={`tls-key-pem-${domain}`}>
              {t('proxySetup.byo.keyLabel')}
            </FieldLabel>
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
                ? t('proxySetup.byo.replaceHint')
                : t('proxySetup.byo.newHint')}
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
              {setCert.isPending
                ? t('proxySetup.byo.uploading')
                : t('proxySetup.byo.upload')}
            </Button>
            {uploaded ? (
              <Dialog
                open={confirmingRevert}
                onOpenChange={setConfirmingRevert}
              >
                <DialogTrigger
                  render={
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={pending}
                    />
                  }
                >
                  {t('proxySetup.byo.revert')}
                </DialogTrigger>
                <DialogContent className="sm:max-w-sm">
                  <DialogHeader>
                    <DialogTitle className="flex items-center gap-1.5 text-destructive">
                      <WarningIcon className="size-4" aria-hidden="true" />
                      {t('proxySetup.byo.revertTitle', { domain })}
                    </DialogTitle>
                    <DialogDescription>
                      {t('proxySetup.byo.revertBody')}
                    </DialogDescription>
                  </DialogHeader>
                  <DialogFooter>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setConfirmingRevert(false)}
                    >
                      {t('proxySetup.byo.cancel')}
                    </Button>
                    <Button
                      type="button"
                      variant="destructive"
                      disabled={clearCert.isPending}
                      onClick={() => {
                        clearCert.mutate(undefined, {
                          onSuccess: () => {
                            setConfirmingRevert(false)
                            toast.add({
                              title: t('proxySetup.byo.revertedToast', {
                                domain,
                              }),
                              type: 'success',
                            })
                          },
                          onError: (error) => {
                            toast.add({
                              title: t('proxySetup.byo.revertFailed'),
                              description: error.message,
                              type: 'error',
                            })
                          },
                        })
                      }}
                    >
                      {clearCert.isPending
                        ? t('proxySetup.byo.reverting')
                        : t('proxySetup.byo.revertConfirm')}
                    </Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            ) : null}
          </div>
        </form>
      ) : null}
    </div>
  )
}
