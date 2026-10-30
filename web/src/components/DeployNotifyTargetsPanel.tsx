import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { RocketGlassIcon } from '@thesvg/ui-react'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Badge } from '@/components/ui/badge'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { CreateDeployNotifyTargetDialog } from './CreateDeployNotifyTargetDialog'
import { DeleteDeployNotifyTargetDialog } from './DeleteDeployNotifyTargetDialog'
import { useDeployNotifyTargets } from '../queries/deployNotify'
import type { DeployNotifyTarget } from '../types/deployNotify'

// Deploy-outcome notifications section for the app detail page (wave-2
// roadmap item #5), wired against GET/POST/DELETE
// /api/v1/apps/{name}/deploy-notify-targets. Sits alongside
// AlertRulesPanel on the same "alerts" route rather than a separate nav
// tab: both panels configure "where does this app send a notification,"
// an operator already on this page understands the boundary between the
// two (threshold/crashloop conditions vs. every deploy finishing) once
// they read each panel's own description, so a second nav item would
// just be one more click to reach a closely related setting.
//
// Same "mask the destination, never print a webhook URL in full" choice
// AlertRulesPanel's own maskNotifyUrl already makes: a Slack/Discord
// incoming-webhook URL embeds a bearer secret in its path.
function maskNotifyUrl(url: string): string {
  try {
    const parsed = new URL(url)
    return `${parsed.host}/••••`
  } catch {
    return '••••'
  }
}

export function DeployNotifyTargetsPanel({ appName }: { appName: string }) {
  const { t } = useTranslation('common')
  const { data, isLoading, error } = useDeployNotifyTargets(appName)
  const targets = data ?? []
  const columns = useMemo<ResponsiveColumn<DeployNotifyTarget>[]>(
    () => [
      {
        id: 'channel',
        header: 'Channel',
        primary: true,
        cell: (target) => (
          <Badge variant="outline">{target.notify_kind ?? 'generic'}</Badge>
        ),
      },
      {
        id: 'destination',
        header: 'Destination',
        cell: (target) => (
          <span
            className="block truncate font-mono text-xs text-muted-foreground"
            title={target.notify_url}
          >
            {target.notify_url ? maskNotifyUrl(target.notify_url) : '-'}
          </span>
        ),
      },
      {
        id: 'status',
        header: 'Status',
        cell: (target) =>
          target.enabled ? (
            <Badge variant="success">Enabled</Badge>
          ) : (
            <Badge variant="muted">Disabled</Badge>
          ),
      },
    ],
    [],
  )

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
            <RocketGlassIcon
              className="size-5 text-muted-foreground"
              aria-hidden="true"
            />
            Deploy notifications
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Notifies via webhook, Slack, Discord, Telegram, or email once per
            deploy attempt that succeeds or fails. Distinct from the
            threshold/crashloop alert rules above.
          </p>
        </div>
        <CreateDeployNotifyTargetDialog appName={appName} />
      </div>

      <div className="mt-3">
        {isLoading ? (
          <TableSkeleton columnCount={4} rowCount={3} />
        ) : error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : targets.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No deploy notify targets yet. Add one to get pinged when a deploy
            succeeds or fails.
          </p>
        ) : (
          <ResponsiveTable
            rows={targets}
            columns={columns}
            getRowId={(target) => target.id}
            ariaLabel={t('tableLabels.deployNotifyTargets')}
            renderActions={(target) => (
              <DeleteDeployNotifyTargetDialog
                appName={appName}
                target={target}
              />
            )}
          />
        )}
      </div>
    </section>
  )
}
