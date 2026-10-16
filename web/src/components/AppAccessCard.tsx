import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  ArrowSquareOutIcon,
  CheckIcon,
  CopyIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import {
  useCheckExposure,
  usePreviewExposure,
  useSetExposure,
  type ExposureCheck,
  type ExposurePlan,
  type ProbeTarget,
} from '../queries/appExposure'
import { useAppUrls } from '../queries/appUrls'
import {
  deriveAccessRows,
  type AccessRow,
  type AccessState,
} from './appAccessRows'

const DOT: Record<AccessState, string> = {
  ok: 'bg-tone-success-solid',
  warn: 'bg-tone-warning-solid',
  off: 'bg-muted-foreground/40',
  unknown: 'bg-muted-foreground/40',
}

function probeFor(
  row: AccessRow,
  check?: ExposureCheck,
): ProbeTarget | undefined {
  if (!check || !row.href) return undefined
  return check.targets.find((t) => t.url.startsWith(row.href as string))
}

function AccessRowView({
  row,
  probe,
  testing,
  onTest,
  onAllow,
}: {
  row: AccessRow
  probe?: ProbeTarget
  testing: boolean
  onTest: () => void
  onAllow: () => void
}) {
  const { t } = useTranslation('appAccess')
  const { copied, copy } = useCopyToClipboard()
  const what = t(`id.${row.id}`)
  const state: AccessState = probe ? (probe.ok ? 'ok' : 'warn') : row.state
  const probeNote = probe
    ? probe.ok
      ? t('test.ok', { ms: probe.latency_ms ?? 0 })
      : probe.detail
        ? t('test.failedWith', { detail: probe.detail })
        : t('test.failed')
    : null
  return (
    <li className="flex flex-col gap-1 py-2 sm:flex-row sm:items-center sm:gap-3">
      <div className="flex min-w-0 items-center gap-2 sm:w-40 sm:shrink-0">
        <span
          role="img"
          aria-label={t(`state.${state}`)}
          className={`size-2 shrink-0 rounded-full ${DOT[state]}`}
        />
        <span className="text-sm font-medium">{what}</span>
      </div>
      <div className="min-w-0 flex-1">
        {row.display ? (
          <code className="block truncate font-mono text-sm">
            {row.display}
          </code>
        ) : null}
        <p className="text-xs text-muted-foreground">
          {probeNote ?? t(`note.${row.note}`)}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {row.allowNetwork ? (
          <Button size="sm" variant="outline" onClick={onAllow}>
            <WarningIcon className="size-3.5" aria-hidden="true" />
            {t('actions.allowNetwork')}
          </Button>
        ) : null}
        {row.href ? (
          <Button size="sm" variant="ghost" disabled={testing} onClick={onTest}>
            {testing ? t('actions.testing') : t('actions.test')}
          </Button>
        ) : null}
        {row.value ? (
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={
              copied ? t('actions.copied') : t('actions.copy', { what })
            }
            onClick={() => copy(row.value)}
          >
            {copied ? (
              <CheckIcon className="size-3.5" aria-hidden="true" />
            ) : (
              <CopyIcon className="size-3.5" aria-hidden="true" />
            )}
          </Button>
        ) : null}
        {row.href ? (
          <a
            href={row.href}
            target="_blank"
            rel="noreferrer noopener"
            aria-label={t('actions.open', { what })}
            className={buttonVariants({ size: 'icon-sm', variant: 'ghost' })}
          >
            <ArrowSquareOutIcon className="size-3.5" aria-hidden="true" />
          </a>
        ) : null}
      </div>
    </li>
  )
}

function AllowDialog({
  appName,
  plan,
  onClose,
}: {
  appName: string
  plan: ExposurePlan | null
  onClose: () => void
}) {
  const { t } = useTranslation('appAccess')
  const { t: tx } = useTranslation('appExposure')
  const apply = useSetExposure(appName)
  const lines = plan
    ? [
        ...plan.changes.map((c) =>
          t('allow.change', {
            what: tx(`change.${c.kind}`, { defaultValue: c.kind }),
            from: c.from ?? '',
            to: c.to ?? '',
          }),
        ),
        ...(plan.warnings ?? []).map((w) =>
          t(`allow.${w}`, { defaultValue: w }),
        ),
        ...(plan.recreates ? [t('allow.recreates')] : []),
      ]
    : []
  return (
    <ConfirmDialog
      open={plan !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
      title={t('allow.title')}
      description={
        plan && plan.changes.length === 0
          ? t('allow.none')
          : t('allow.description')
      }
      consequences={lines}
      confirmLabel={t('allow.confirm')}
      pendingLabel={t('allow.pending')}
      pending={apply.isPending}
      error={apply.error?.message ?? null}
      onConfirm={() =>
        apply.mutate(
          { mode: 'direct' },
          {
            onSuccess: () => {
              toast.add({
                title: t('allow.done'),
                description: t('allow.doneDescription'),
                type: 'success',
              })
              onClose()
            },
          },
        )
      }
    />
  )
}

/** Where this app can be reached right now, on which port, with tests. */
export function AppAccessCard({ appName }: { appName: string }) {
  const { t } = useTranslation('appAccess')
  const { data, isError, isPending, refetch } = useAppUrls(appName)
  const check = useCheckExposure(appName)
  const preview = usePreviewExposure(appName)
  const [plan, setPlan] = useState<ExposurePlan | null>(null)

  const askAllow = () =>
    preview.mutate(
      { mode: 'direct' },
      {
        onSuccess: setPlan,
        onError: (e) => toast.add({ title: e.message, type: 'error' }),
      },
    )

  return (
    <Card aria-label={t('title')}>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
      </CardHeader>
      <CardContent>
        {isPending ? (
          <div className="space-y-2" aria-hidden="true">
            {Array.from({ length: 4 }, (_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : isError || !data ? (
          <Alert variant="destructive">
            <AlertDescription className="flex flex-wrap items-center gap-2">
              {t('errors.load')}
              <Button
                size="sm"
                variant="outline"
                onClick={() => void refetch()}
              >
                {t('errors.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : (
          <ul className="divide-y divide-border">
            {deriveAccessRows(data).map((row) => (
              <AccessRowView
                key={row.key}
                row={row}
                probe={probeFor(row, check.data)}
                testing={check.isPending}
                onTest={() => check.mutate()}
                onAllow={askAllow}
              />
            ))}
          </ul>
        )}
        <div className="mt-3">
          <Link
            to="/apps/$name/network"
            params={{ name: appName }}
            className="text-sm text-primary underline underline-offset-2"
          >
            {t('actions.change')}
          </Link>
        </div>
      </CardContent>
      <AllowDialog
        appName={appName}
        plan={plan}
        onClose={() => setPlan(null)}
      />
    </Card>
  )
}
