import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { ApiError } from '../lib/apiError'
import {
  auditAnchorStatusQueryOptions,
  useCheckAuditAnchor,
} from '../queries/auditLog'

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <span className="inline-flex items-baseline gap-1">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium tabular-nums">{value}</span>
    </span>
  )
}

export function AuditAnchorCard() {
  const { t } = useTranslation('auditLog')
  const { data, error } = useQuery(auditAnchorStatusQueryOptions())
  const check = useCheckAuditAnchor()

  if (error instanceof ApiError && [403, 404].includes(error.status)) {
    return null
  }
  if (!data) return null

  return (
    <section
      aria-label={t('anchor.title')}
      className="space-y-2 rounded-lg border bg-card px-3 py-2 text-sm"
    >
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
        <span className="inline-flex items-center gap-1.5 font-medium">
          <ShieldCheckGlassIcon size="1.2em" aria-hidden="true" />
          {t('anchor.title')}
        </span>
        {data.enabled ? (
          <>
            <Badge variant="success">{t('anchor.exporting')}</Badge>
            <span className="text-muted-foreground">
              {data.last_exported_at
                ? `${t('anchor.lastExported', { seq: data.last_exported_seq })} ${t('anchor.at', { time: new Date(data.last_exported_at).toLocaleString() })}`
                : t('anchor.never')}
            </span>
            <Stat label={t('anchor.exported')} value={data.exported} />
            <Stat label={t('anchor.failed')} value={data.failed} />
            <Stat label={t('anchor.dropped')} value={data.dropped} />
            <Stat label={t('anchor.queued')} value={data.queued} />
          </>
        ) : (
          <>
            <Badge variant="muted">{t('anchor.off')}</Badge>
            <span className="text-muted-foreground">{t('anchor.offHint')}</span>
            <Link
              to="/settings/security-runtime"
              className={buttonVariants({ variant: 'link', size: 'sm' })}
            >
              {t('anchor.offLink')}
            </Link>
          </>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={check.isPending}
          onClick={() => {
            check.mutate()
          }}
        >
          {check.isPending ? t('anchor.checking') : t('anchor.check')}
        </Button>
      </div>
      {check.data?.ok && check.data.matched > 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('anchor.checkOk', { seq: check.data.live_head })}
        </p>
      ) : null}
      {check.data?.ok && check.data.matched === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('anchor.checkCompacted')}
        </p>
      ) : null}
      {check.data && !check.data.ok ? (
        <Alert role="alert" variant="destructive">
          <TriangleAlertGlassIcon size="1.2em" aria-hidden="true" />
          <AlertDescription>{t('anchor.checkMismatch')}</AlertDescription>
        </Alert>
      ) : null}
      {check.isError ? (
        <p className="text-sm text-destructive">
          {t('anchor.checkFailed')}: {check.error.message}
        </p>
      ) : null}
      {data.enabled && data.last_error ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon size="1.2em" aria-hidden="true" />
          <AlertTitle>{t('anchor.lastError')}</AlertTitle>
          <AlertDescription>
            {data.last_error}
            {data.last_error_at
              ? ` ${t('anchor.at', { time: new Date(data.last_error_at).toLocaleString() })}`
              : ''}
          </AlertDescription>
        </Alert>
      ) : null}
    </section>
  )
}
