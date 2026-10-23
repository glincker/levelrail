import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  CheckCircleIcon,
  LockKeyIcon,
  MinusCircleIcon,
  SpinnerGapIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  certPreflightQueryOptions,
  certStatusQueryOptions,
  useGetCertificate,
} from '../queries/domainCert'
import type {
  CertPreflight,
  DomainCertStatus,
  PreflightItem,
} from '../queries/domainCert'

const PORTS_CODE = 'ports'

function ItemIcon({ state }: { state: PreflightItem['state'] }) {
  const cls = 'mt-0.5 size-4 shrink-0'
  if (state === 'pass')
    return (
      <CheckCircleIcon
        className={`${cls} text-tone-success`}
        weight="fill"
        aria-hidden="true"
      />
    )
  if (state === 'fail')
    return (
      <WarningCircleIcon
        className={`${cls} text-tone-danger`}
        weight="fill"
        aria-hidden="true"
      />
    )
  if (state === 'warn')
    return (
      <WarningIcon className={`${cls} text-tone-warning`} aria-hidden="true" />
    )
  return (
    <MinusCircleIcon
      className={`${cls} text-muted-foreground`}
      aria-hidden="true"
    />
  )
}

function formatDate(iso?: string): string {
  return iso ? new Date(iso).toLocaleDateString() : ''
}

function PreflightList({ items }: { items: PreflightItem[] }) {
  const { t } = useTranslation('https')
  return (
    <ul className="space-y-2" aria-label={t('domainCert.checking')}>
      {items.map((it) => (
        <li key={it.code} className="flex items-start gap-2 text-sm">
          <ItemIcon state={it.state} />
          <div className="min-w-0">
            <p className="font-medium">
              {t(`domainCert.item.${it.code}`, { defaultValue: it.code })}
              <span className="sr-only">
                {` (${t(`domainCert.state.${it.state}`)})`}
              </span>
            </p>
            <p className="text-xs text-muted-foreground">{it.detail}</p>
            {it.fix && it.state !== 'pass' ? (
              <p className="text-xs">{t('domainCert.next', { fix: it.fix })}</p>
            ) : null}
          </div>
        </li>
      ))}
    </ul>
  )
}

function Progress({
  status,
  onRetry,
  onClose,
}: {
  status: DomainCertStatus
  onRetry: () => void
  onClose: () => void
}) {
  const { t } = useTranslation('https')
  if (status.state === 'issued') {
    return (
      <div className="space-y-3" role="status">
        <p className="flex items-center gap-2 text-sm font-medium">
          <CheckCircleIcon className="size-4 text-tone-success" weight="fill" />
          {t('domainCert.progress.issuedTitle')}
        </p>
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          <dt className="text-muted-foreground">
            {t('domainCert.progress.issuer')}
          </dt>
          <dd>{status.issuer}</dd>
          <dt className="text-muted-foreground">
            {t('domainCert.progress.expires')}
          </dt>
          <dd>
            {formatDate(status.not_after)}
            {status.days_left === undefined
              ? ''
              : ` (${t('domainCert.badge.days', { count: status.days_left })})`}
          </dd>
          {status.renewal_at ? (
            <>
              <dt className="text-muted-foreground">
                {t('domainCert.progress.renews')}
              </dt>
              <dd>{formatDate(status.renewal_at)}</dd>
            </>
          ) : null}
        </dl>
        {status.staging ? (
          <Alert>
            <WarningIcon />
            <AlertDescription>
              {t('domainCert.progress.stagingIssued')}
            </AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button type="button" onClick={onClose}>
            {t('domainCert.progress.close')}
          </Button>
        </DialogFooter>
      </div>
    )
  }
  if (status.state === 'failed') {
    return (
      <div className="space-y-3">
        <Alert variant="destructive">
          <WarningIcon />
          <AlertTitle>{t('domainCert.progress.failedTitle')}</AlertTitle>
          <AlertDescription>
            <p>{status.explanation}</p>
            {status.error ? (
              <p className="mt-1 text-xs break-words">
                {t('domainCert.progress.caSaid')}:{' '}
                <span className="font-mono">{status.error}</span>
              </p>
            ) : null}
            {status.next_safe_at ? (
              <p className="mt-1 text-xs">
                {t('domainCert.nextSafe', {
                  date: new Date(status.next_safe_at).toLocaleString(),
                })}
              </p>
            ) : null}
          </AlertDescription>
        </Alert>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('domainCert.progress.close')}
          </Button>
          <Button type="button" onClick={onRetry}>
            {t('domainCert.progress.tryAgain')}
          </Button>
        </DialogFooter>
      </div>
    )
  }
  return (
    <div className="space-y-2" role="status">
      <p className="flex items-center gap-2 text-sm font-medium">
        <SpinnerGapIcon className="size-4 animate-spin" aria-hidden="true" />
        {t('domainCert.progress.pendingTitle')}
      </p>
      <p className="text-sm text-muted-foreground">
        {t('domainCert.progress.pendingBody', { domain: status.domain })}
      </p>
    </div>
  )
}

function onlyPortsFailed(p: CertPreflight): boolean {
  return p.items.every((i) => i.state !== 'fail' || i.code === PORTS_CODE)
}

function Review({
  domain,
  onRequested,
}: {
  domain: string
  onRequested: () => void
}) {
  const { t } = useTranslation('https')
  const [typed, setTyped] = useState<string | null>(null)
  const [applied, setApplied] = useState('')
  const [staging, setStaging] = useState(false)
  const [ack, setAck] = useState(false)
  const pre = useQuery(certPreflightQueryOptions(domain, applied, staging))
  const get = useGetCertificate(domain)
  const data = pre.data
  const email = typed ?? data?.email ?? ''
  const portsOnly = data ? !data.ready && onlyPortsFailed(data) : false
  const canSubmit =
    !!data && (data.ready || (portsOnly && ack)) && !get.isPending
  function submit() {
    get.mutate(
      {
        email: typed?.trim() || undefined,
        staging,
        acknowledge: portsOnly && ack ? [PORTS_CODE] : undefined,
      },
      { onSuccess: onRequested },
    )
  }
  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        {t('domainCert.intro', { domain })}
      </p>
      {pre.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>
            {t('domainCert.checkFailed')} {pre.error.message}
          </AlertDescription>
        </Alert>
      ) : data ? (
        <PreflightList items={data.items} />
      ) : (
        <p className="flex items-center gap-2 text-sm" role="status">
          <SpinnerGapIcon className="size-4 animate-spin" aria-hidden="true" />
          {t('domainCert.checking')}
        </p>
      )}
      {data?.next_safe_at ? (
        <p className="text-xs">
          {t('domainCert.nextSafe', {
            date: new Date(data.next_safe_at).toLocaleString(),
          })}
        </p>
      ) : null}
      <div className="max-w-sm space-y-1.5">
        <Label htmlFor="cert-flow-email">{t('domainCert.emailLabel')}</Label>
        <Input
          id="cert-flow-email"
          type="email"
          autoComplete="email"
          value={email}
          onChange={(e) => setTyped(e.target.value)}
          onBlur={() => setApplied((typed ?? '').trim())}
        />
        <p className="text-xs text-muted-foreground">
          {t('domainCert.emailHelp')}
        </p>
      </div>
      <div className="flex items-start gap-2">
        <Checkbox
          id="cert-flow-staging"
          checked={staging}
          onCheckedChange={(v) => setStaging(v === true)}
        />
        <div className="space-y-0.5">
          <Label htmlFor="cert-flow-staging">
            {t('domainCert.stagingLabel')}
          </Label>
          <p className="text-xs text-muted-foreground">
            {t('domainCert.stagingHelp')}
          </p>
        </div>
      </div>
      {staging ? (
        <Alert>
          <WarningIcon />
          <AlertDescription>{t('domainCert.stagingWarning')}</AlertDescription>
        </Alert>
      ) : null}
      {portsOnly ? (
        <div className="flex items-start gap-2">
          <Checkbox
            id="cert-flow-ack"
            checked={ack}
            onCheckedChange={(v) => setAck(v === true)}
          />
          <Label htmlFor="cert-flow-ack">{t('domainCert.ackPorts')}</Label>
        </div>
      ) : null}
      <p className="text-xs text-muted-foreground">
        {t('domainCert.appliesToAll')}
      </p>
      {get.isError ? (
        <p className="text-xs text-destructive" role="alert">
          {t('domainCert.requestFailed')} {get.error.message}
        </p>
      ) : null}
      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={() => {
            setApplied((typed ?? '').trim())
            void pre.refetch()
          }}
          disabled={pre.isFetching}
        >
          {t('domainCert.recheck')}
        </Button>
        <Button type="button" onClick={submit} disabled={!canSubmit}>
          {get.isPending ? t('domainCert.requesting') : t('domainCert.submit')}
        </Button>
      </DialogFooter>
    </div>
  )
}

function Flow({ domain, onClose }: { domain: string; onClose: () => void }) {
  const [requested, setRequested] = useState(false)
  const status = useQuery(certStatusQueryOptions(domain, requested))
  if (requested && status.data) {
    return (
      <Progress
        status={status.data}
        onRetry={() => setRequested(false)}
        onClose={onClose}
      />
    )
  }
  return <Review domain={domain} onRequested={() => setRequested(true)} />
}

/** GetCertificateButton is the one guided action that ends a browser warning: preflight, request, progress. */
export function GetCertificateButton({
  domain,
  variant = 'outline',
}: {
  domain: string
  variant?: 'outline' | 'default' | 'ghost'
}) {
  const { t } = useTranslation('https')
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button
        type="button"
        size="sm"
        variant={variant}
        onClick={() => setOpen(true)}
      >
        <LockKeyIcon className="size-3.5" aria-hidden="true" />
        {t('domainCert.action')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('domainCert.title', { domain })}</DialogTitle>
            <DialogDescription className="sr-only">
              {t('domainCert.intro', { domain })}
            </DialogDescription>
          </DialogHeader>
          {open ? (
            <Flow domain={domain} onClose={() => setOpen(false)} />
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  )
}
