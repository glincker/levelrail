import { useQueries } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleGlassIcon,
  CircleMinusGlassIcon,
  LoaderCircleGlassIcon,
} from '@thesvg/ui-react'
import { useAlertRules } from '../../queries/alerts'
import { useEnvironmentListOptional } from '../../queries/environments'
import { useProjectListOptional } from '../../queries/projects'
import { volumeBackupScheduleQueryOptions } from '../../queries/volumeBackupSchedule'
import type { AppDetail } from '../../types/appDetail'
import {
  computeKeepChecklist,
  type KeepItem,
  type KeepItemId,
  type KeepItemState,
} from './keepChecklist'

const TARGET: Record<
  KeepItemId,
  | '/apps/$name/domains'
  | '/apps/$name/volumes'
  | '/apps/$name/alerts'
  | '/apps/$name/environment'
> = {
  domain: '/apps/$name/domains',
  backup: '/apps/$name/volumes',
  alert: '/apps/$name/alerts',
  placement: '/apps/$name/environment',
}

type ItemKey<K extends string> = `dialog.items.${KeepItemId}.${K}`

const LABEL_KEY: Record<KeepItemId, ItemKey<'label'>> = {
  domain: 'dialog.items.domain.label',
  backup: 'dialog.items.backup.label',
  alert: 'dialog.items.alert.label',
  placement: 'dialog.items.placement.label',
}

const ACTION_KEY: Record<KeepItemId, ItemKey<'action'>> = {
  domain: 'dialog.items.domain.action',
  backup: 'dialog.items.backup.action',
  alert: 'dialog.items.alert.action',
  placement: 'dialog.items.placement.action',
}

type StatusKey =
  | ItemKey<'done'>
  | ItemKey<'todo'>
  | 'dialog.items.backup.none'
  | 'dialog.items.placement.projectOnly'
  | 'dialog.loading'

function statusKey(item: KeepItem): StatusKey {
  if (item.state === 'loading') return 'dialog.loading'
  if (item.state === 'none') return 'dialog.items.backup.none'
  if (item.variant === 'projectOnly')
    return 'dialog.items.placement.projectOnly'
  const done: Record<KeepItemId, ItemKey<'done'>> = {
    domain: 'dialog.items.domain.done',
    backup: 'dialog.items.backup.done',
    alert: 'dialog.items.alert.done',
    placement: 'dialog.items.placement.done',
  }
  const todo: Record<KeepItemId, ItemKey<'todo'>> = {
    domain: 'dialog.items.domain.todo',
    backup: 'dialog.items.backup.todo',
    alert: 'dialog.items.alert.todo',
    placement: 'dialog.items.placement.todo',
  }
  return item.state === 'done' ? done[item.id] : todo[item.id]
}

const STATE_ICON: Record<KeepItemState, typeof CircleGlassIcon> = {
  done: CircleCheckGlassIcon,
  todo: CircleGlassIcon,
  loading: LoaderCircleGlassIcon,
  none: CircleMinusGlassIcon,
}

const STATE_CLASS: Record<KeepItemState, string> = {
  done: 'text-tone-success',
  todo: 'text-muted-foreground',
  loading: 'animate-spin text-muted-foreground motion-reduce:animate-none',
  none: 'text-muted-foreground',
}

// Live status of what a permanent app should have, each linked to the
// screen that sets it up. Mounted only while the keep dialog is open.
export function KeepTrialChecklist({
  app,
}: {
  app: Pick<
    AppDetail,
    'name' | 'domains' | 'volumes' | 'project_id' | 'environment_id'
  >
}) {
  const { t } = useTranslation('trialApp')
  const volumes = app.volumes ?? []
  const schedules = useQueries({
    queries: volumes.map((v) => ({
      ...volumeBackupScheduleQueryOptions(app.name, v.name),
      retry: false,
    })),
  })
  const alerts = useAlertRules(app.name)
  const projects = useProjectListOptional()
  const environments = useEnvironmentListOptional(app.project_id ?? '')
  const project = projects.data?.find((p) => p.id === app.project_id)
  const environment = environments.data?.find(
    (e) => e.id === app.environment_id,
  )

  const items = computeKeepChecklist({
    domains: app.domains,
    volumeSchedules: schedules.map((q) => {
      if (q.isError) return false
      return q.data === undefined ? undefined : Boolean(q.data.schedule)
    }),
    alertCount: alerts.isError ? 0 : alerts.data?.length,
    projectName: project?.name ?? (app.project_id ? app.project_id : undefined),
    environmentName: environment?.name,
  })

  return (
    <section className="space-y-2">
      <div>
        <h3 className="text-sm font-medium text-foreground">
          {t('dialog.checklistTitle')}
        </h3>
        <p className="text-xs text-muted-foreground">
          {t('dialog.checklistHint')}
        </p>
      </div>
      <ul className="divide-y divide-border rounded-lg border border-border">
        {items.map((item) => {
          const Icon = STATE_ICON[item.state]
          return (
            <li
              key={item.id}
              className="flex items-start justify-between gap-3 px-3 py-2"
            >
              <div className="flex min-w-0 items-start gap-2">
                <Icon
                  className={`mt-0.5 size-4 shrink-0 ${STATE_CLASS[item.state]}`}
                  aria-label={t(
                    item.state === 'done' ? 'dialog.done' : 'dialog.todo',
                  )}
                />
                <div className="min-w-0 text-sm">
                  <p className="text-foreground">{t(LABEL_KEY[item.id])}</p>
                  <p className="text-xs break-words text-muted-foreground">
                    {t(statusKey(item), item.values)}
                  </p>
                </div>
              </div>
              {item.state === 'todo' ? (
                <Link
                  to={TARGET[item.id]}
                  params={{ name: app.name }}
                  className="shrink-0 text-xs font-medium text-primary underline-offset-2 hover:underline"
                >
                  {t(ACTION_KEY[item.id])}
                </Link>
              ) : null}
            </li>
          )
        })}
      </ul>
    </section>
  )
}
