import { RocketLaunchIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
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

function TargetRow({
  appName,
  target,
}: {
  appName: string
  target: DeployNotifyTarget
}) {
  return (
    <TableRow>
      <TableCell>
        <Badge variant="outline">{target.notify_kind ?? 'generic'}</Badge>
      </TableCell>
      <TableCell
        className="max-w-[20rem] truncate font-mono text-xs text-muted-foreground"
        title={target.notify_url}
      >
        {target.notify_url ? maskNotifyUrl(target.notify_url) : '-'}
      </TableCell>
      <TableCell>
        {target.enabled ? (
          <Badge variant="success">Enabled</Badge>
        ) : (
          <Badge variant="muted">Disabled</Badge>
        )}
      </TableCell>
      <TableCell className="text-right">
        <DeleteDeployNotifyTargetDialog appName={appName} target={target} />
      </TableCell>
    </TableRow>
  )
}

export function DeployNotifyTargetsPanel({ appName }: { appName: string }) {
  const { data, isLoading, error } = useDeployNotifyTargets(appName)
  const targets = data ?? []

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <RocketLaunchIcon
              className="size-4 text-muted-foreground"
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
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : targets.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No deploy notify targets yet. Add one to get pinged when a deploy
            succeeds or fails.
          </p>
        ) : (
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Channel</TableHead>
                  <TableHead>Destination</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {targets.map((target) => (
                  <TargetRow key={target.id} appName={appName} target={target} />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </section>
  )
}
