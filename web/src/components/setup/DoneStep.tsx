import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightIcon,
  BellRingingIcon,
  CheckCircleIcon,
  CloudArrowUpIcon,
  EnvelopeSimpleIcon,
  MinusCircleIcon,
  PackageIcon,
  UsersIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Button, buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useBrand } from '../../hooks/useBrand'
import { dashboardUrlQueryOptions } from '../../queries/dashboardUrl'
import { SETUP_STEPS } from '../../lib/setupWizard'
import type { SetupStepId, SetupStepMap } from '../../lib/setupWizard'
import { CopyValue } from './StepChrome'

interface NextAction {
  to:
    | '/settings/email'
    | '/settings/notification-channels'
    | '/settings/backup-targets'
    | '/settings/users'
  prefix: 'email' | 'alerts' | 'backups' | 'team'
  icon: Icon
}

const NEXT_ACTIONS: readonly NextAction[] = [
  { to: '/settings/email', prefix: 'email', icon: EnvelopeSimpleIcon },
  { to: '/settings/backup-targets', prefix: 'backups', icon: CloudArrowUpIcon },
  {
    to: '/settings/notification-channels',
    prefix: 'alerts',
    icon: BellRingingIcon,
  },
  { to: '/settings/users', prefix: 'team', icon: UsersIcon },
]

/** DoneStep is the completion moment: what was configured, the dashboard address, and three concrete next actions. */
export function DoneStep({
  steps,
  onGoToStep,
  onFinish,
  pending,
}: {
  steps: SetupStepMap
  onGoToStep: (id: SetupStepId) => void
  onFinish: () => void
  pending: boolean
}) {
  const { t } = useTranslation('setup')
  const { t: ts } = useTranslation('settings')
  const brand = useBrand()
  const { data: dashboard } = useQuery(dashboardUrlQueryOptions())
  const reviewable = SETUP_STEPS.filter((id) => id !== 'done')
  const skipped = reviewable.filter((id) => steps[id] !== 'completed').length
  const address = dashboard?.dashboard_url || window.location.origin

  // Email is only worth suggesting when it was skipped; otherwise the first three remaining.
  const actions = NEXT_ACTIONS.filter(
    (a) => a.prefix !== 'email' || steps.email !== 'completed',
  ).slice(0, 3)

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <p className="flex items-center gap-1.5 text-xs font-medium uppercase tracking-wider text-tone-success">
          <CheckCircleIcon className="size-4" aria-hidden="true" />
          {t('done.eyebrow')}
        </p>
        <h3 className="text-xl font-semibold tracking-tight text-foreground">
          {skipped === 0
            ? t('done.title', { name: brand.Name })
            : t('done.titleSkipped', { name: brand.Name })}
        </h3>
        <p className="text-sm text-muted-foreground">{t('done.subtitle')}</p>
      </div>

      <section className="space-y-2">
        <h4 className="text-sm font-medium text-foreground">
          {t('done.summaryHeading')}
        </h4>
        <ul className="divide-y divide-border rounded-xl border border-border">
          {reviewable.map((id) => {
            const done = steps[id] === 'completed'
            return (
              <li
                key={id}
                className="flex items-center justify-between gap-3 px-3 py-2.5"
              >
                <span className="flex min-w-0 items-center gap-2 text-sm text-foreground">
                  {done ? (
                    <CheckCircleIcon
                      className="size-5 shrink-0 text-tone-success"
                      aria-hidden="true"
                    />
                  ) : (
                    <MinusCircleIcon
                      className="size-5 shrink-0 text-muted-foreground"
                      aria-hidden="true"
                    />
                  )}
                  <span className="truncate">{t(`steps.${id}.title`)}</span>
                  <span className="text-xs text-muted-foreground">
                    {done
                      ? t('done.completed')
                      : steps[id] === 'skipped'
                        ? t('done.skipped')
                        : t('done.notStarted')}
                  </span>
                </span>
                {done ? null : (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => onGoToStep(id)}
                  >
                    {t('done.goBack')}
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      </section>

      <section className="space-y-1.5">
        <h4 className="text-sm font-medium text-foreground">
          {t('done.dashboardUrl')}
        </h4>
        <CopyValue value={address} label={t('done.dashboardUrl')} />
      </section>

      <section className="space-y-2">
        <h4 className="text-sm font-medium text-foreground">
          {t('done.nextHeading')}
        </h4>
        <div className="grid gap-2 sm:grid-cols-3">
          {actions.map((a) => {
            const NextIcon = a.icon
            return (
              <Link
                key={a.to}
                to={a.to}
                className="group space-y-1 rounded-xl border border-border p-3 outline-none transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 motion-reduce:transition-none"
              >
                <NextIcon
                  className="size-5 text-muted-foreground"
                  aria-hidden="true"
                />
                <p className="text-sm font-medium text-foreground">
                  {ts(`setupNext.${a.prefix}.title`)}
                </p>
                <p className="text-xs text-muted-foreground">
                  {ts(`setupNext.${a.prefix}.description`)}
                </p>
              </Link>
            )
          })}
        </div>
      </section>

      <section className="flex flex-col gap-3 rounded-xl border border-tone-accent-border bg-tone-accent-soft/40 p-4 sm:flex-row sm:items-center">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-tone-accent-soft text-tone-accent">
          <PackageIcon className="size-5" aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <h4 className="text-sm font-medium text-foreground">
            {t('done.importTitle')}
          </h4>
          <p className="text-xs text-muted-foreground">
            {t('done.importBody')}
          </p>
        </div>
        <Link
          to="/settings/import-platform"
          className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
        >
          {t('done.importAction')}
          <ArrowRightIcon />
        </Link>
      </section>

      <div className="flex justify-end border-t border-border pt-4">
        <Button onClick={onFinish} disabled={pending}>
          {t('done.finish')}
        </Button>
      </div>
    </div>
  )
}
