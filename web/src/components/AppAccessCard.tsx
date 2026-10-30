import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  useCheckExposure,
  usePreviewExposure,
  useSetExposure,
  type ExposureCheck,
  type ExposurePlan,
  type ProbeTarget,
} from '../queries/appExposure'
import { useAppUrls } from '../queries/appUrls'
import { ReachAdvanced } from './appAccess/ReachAdvanced'
import { ReachHero } from './appAccess/ReachHero'
import { ReachRow } from './appAccess/ReachRow'
import { deriveReach, type ReachAddress } from './appAccess/reachModel'

function probeFor(
  addr: ReachAddress,
  check?: ExposureCheck,
): ProbeTarget | undefined {
  if (!check || !addr.href) return undefined
  const href = addr.href
  return check.targets.find((t) => t.url.startsWith(href))
}

// The container probe gates the rest, so its failure explains the hero too.
function heroProbeFor(
  addr: ReachAddress,
  check?: ExposureCheck,
): ProbeTarget | undefined {
  const own = probeFor(addr, check)
  if (own) return own
  const base = check?.targets.find((t) => t.kind === 'container')
  return base && !base.ok ? base : undefined
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

/** Where this app can be reached right now, best address first. */
export function AppAccessCard({ appName }: { appName: string }) {
  const { t } = useTranslation('appAccess')
  const { data, isError, isPending, refetch } = useAppUrls(appName)
  const check = useCheckExposure(appName)
  const preview = usePreviewExposure(appName)
  const [plan, setPlan] = useState<ExposurePlan | null>(null)
  const autoChecked = useRef(false)
  const { mutate: runCheck } = check
  const canCheck = Boolean(data?.running && data.local_node)

  useEffect(() => {
    if (!canCheck || autoChecked.current) return
    autoChecked.current = true
    runCheck()
  }, [canCheck, runCheck])

  const askAllow = () =>
    preview.mutate(
      { mode: 'direct' },
      {
        onSuccess: setPlan,
        onError: (e) => toast.add({ title: e.message, type: 'error' }),
      },
    )

  const model = data ? deriveReach(data, check.data?.targets) : null
  const row = (addr: ReachAddress) => (
    <ReachRow
      key={addr.key}
      addr={addr}
      appName={appName}
      probe={probeFor(addr, check.data)}
      testing={check.isPending}
      onTest={() => check.mutate()}
      onAllow={askAllow}
    />
  )

  return (
    <Card aria-label={t('title')}>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
      </CardHeader>
      <CardContent className="space-y-4">
        {isPending ? (
          <div className="space-y-2" aria-hidden="true">
            {Array.from({ length: 3 }, (_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : isError || !model ? (
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
          <>
            {model.hero ? (
              <ReachHero
                addr={model.hero}
                appName={appName}
                running={model.running}
                probe={heroProbeFor(model.hero, check.data)}
                checking={check.isPending}
                onRecheck={() => check.mutate()}
                onAllow={askAllow}
              />
            ) : null}
            {model.demoted ? (
              <p className="text-xs text-muted-foreground">
                {t('advanced.autoDown')}
              </p>
            ) : null}
            {model.promoted ? (
              <p className="text-xs text-muted-foreground">
                {t('advanced.noPublic')}
              </p>
            ) : null}
            {model.others.length > 0 ? (
              <section aria-label={t('others.title')}>
                <h3 className="text-sm font-semibold">{t('others.title')}</h3>
                <ul className="divide-y divide-border">
                  {model.others.map(row)}
                </ul>
              </section>
            ) : null}
            <ReachAdvanced defaultOpen={model.autoExpand}>
              {model.advanced.map(row)}
            </ReachAdvanced>
          </>
        )}
      </CardContent>
      <AllowDialog
        appName={appName}
        plan={plan}
        onClose={() => setPlan(null)}
      />
    </Card>
  )
}
