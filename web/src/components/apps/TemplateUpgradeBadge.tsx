import { useState } from 'react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { ArrowFatLinesUpIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  useApplyTemplateUpgrade,
  useTemplateUpgradePlan,
  useTemplateUpgradeRun,
  type UpgradeServicePlan,
} from '../../queries/templateUpgrade'

function serviceLine(s: UpgradeServicePlan, t: TFunction<'deploys'>): string {
  const parts: string[] = []
  if (s.image?.from && s.image.to) {
    parts.push(`${s.image.from} -> ${s.image.to}`)
  } else if (s.image?.to) {
    parts.push(s.image.to)
  }
  const env = (s.env ?? []).filter((e) => e.action !== 'keep_user').length
  if (env > 0) parts.push(t('templateUpgrade.envCount', { count: env }))
  if (s.volumes?.length) {
    parts.push(t('templateUpgrade.volumeCount', { count: s.volumes.length }))
  }
  const fallback = t(
    `templateUpgrade.status.${s.status}` as 'templateUpgrade.status.changed',
  )
  return `${s.name}: ${parts.join(', ') || fallback}`
}

/** Shows when the catalog template an app came from has moved on, and applies it. */
export function TemplateUpgradeBadge({
  appName,
}: Readonly<{ appName: string }>) {
  const { t } = useTranslation('deploys')
  const [open, setOpen] = useState(false)
  const { data } = useTemplateUpgradePlan(appName)
  const apply = useApplyTemplateUpgrade(appName)
  const { data: run } = useTemplateUpgradeRun(
    appName,
    apply.isSuccess || apply.isPending,
  )
  const active = run ?? apply.data

  if (active) {
    return (
      <Alert role="status" className="py-1.5">
        <ArrowFatLinesUpIcon className="size-4 shrink-0" aria-hidden="true" />
        <AlertDescription className="flex flex-wrap items-center gap-x-2">
          <span className="font-medium">
            {t(`templateUpgrade.run.${active.state}`)}
          </span>
          {active.message ? (
            <span className="text-muted-foreground">{active.message}</span>
          ) : null}
        </AlertDescription>
      </Alert>
    )
  }
  if (!data?.has_changes) return null

  const blocked = data.blockers.length > 0
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant="info" className="gap-1">
        <ArrowFatLinesUpIcon className="size-3.5" aria-hidden="true" />
        {t('templateUpgrade.badge')}
      </Badge>
      <span className="text-xs text-muted-foreground">
        {t(`templateUpgrade.risk.${data.risk}`)}
        {blocked ? ` · ${t('templateUpgrade.blocked')}` : ''}
      </span>
      <Button
        size="sm"
        variant="outline"
        disabled={blocked}
        onClick={() => {
          setOpen(true)
        }}
      >
        {t('templateUpgrade.apply')}
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={t('templateUpgrade.confirmTitle')}
        description={t('templateUpgrade.confirmBody', {
          risk: t(`templateUpgrade.risk.${data.risk}`),
        })}
        consequences={data.services
          .filter((s) => s.status !== 'unchanged')
          .map((s) => serviceLine(s, t))}
        confirmLabel={t('templateUpgrade.apply')}
        pendingLabel={t('templateUpgrade.applying')}
        pending={apply.isPending}
        error={apply.isError ? apply.error.message : null}
        onConfirm={() => {
          apply.mutate(true, {
            onSuccess: () => {
              setOpen(false)
            },
          })
        }}
      />
    </div>
  )
}
