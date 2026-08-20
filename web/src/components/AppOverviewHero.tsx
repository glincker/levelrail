import {
  ArrowSquareOutIcon,
  CheckCircleIcon,
  GlobeIcon,
  InfoIcon,
  PackageIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Link } from '@tanstack/react-router'
import type { Icon } from '@phosphor-icons/react'
import type { AppDetail } from '../types/appDetail'
import type { ReconcileCondition } from '../types/deploy'
import { summarizeAppStatus } from '../lib/appStatus'
import { formatBytes, formatNanoCpus } from '../lib/format'
import { useAppNetwork } from '../queries/appNetwork'
import { useCloudflareTunnelStatus } from '../queries/cloudflareTunnel'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

// The Overview page's redesigned top section: a hero (name, status,
// domain, image, node) plus a setup checklist, combined into one panel
// per the approved design. Status comes from the same conditions array
// the parent $name.tsx layout route's loader already primed
// (queries/deploys.ts' useDeployStatus, read by overview.tsx and passed
// down here), the same cache AppScopedSidebar's own status badge reads:
// no new fetch happens in this component.
//
// The checklist is config-state only, deliberately: "is a domain/health
// check/node configured" is answered entirely from fields already on
// AppDetail. It does not resolve DNS or probe a TLS cert to confirm the
// domain actually works, that's a real, documented future item, not
// something silently faked here.
export function AppOverviewHero({
  app,
  conditions,
}: {
  app: AppDetail
  conditions: ReconcileCondition[]
}) {
  const status = summarizeAppStatus(conditions)
  const primaryDomain = app.domains?.[0] ?? null
  const hasHealthCheck = Boolean(app.health?.readiness || app.health?.liveness)
  const hasResourceLimits = Boolean(
    app.resources?.memory_bytes || app.resources?.nano_cpus,
  )

  // Both queries are supplementary signals for the "Domain connected"
  // checklist row below, not primary page data: plain (non-suspending)
  // reads so a slow or not-yet-cached network/tunnel fetch never blocks
  // the rest of this already-loaded page, the same restraint
  // queries/domainCheck.ts's useDomainCheck already applies.
  const { data: network } = useAppNetwork(app.name)
  const { data: tunnelStatus } = useCloudflareTunnelStatus()
  const tunnelAvailable = tunnelStatus?.enabled && tunnelStatus.status === 'connected'

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          <PackageIcon className="size-4" />
          {app.name}
          <Badge variant={status.variant}>{status.label}</Badge>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5">
        <div className="flex flex-wrap items-center gap-x-8 gap-y-3 text-sm">
          <HeroField label="Domain">
            {primaryDomain ? (
              <a
                href={`https://${primaryDomain}`}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 text-primary underline underline-offset-2"
              >
                <GlobeIcon className="size-3.5" />
                {primaryDomain}
                <ArrowSquareOutIcon className="size-3.5" />
              </a>
            ) : (
              <span className="text-muted-foreground italic">no domain</span>
            )}
          </HeroField>
          <HeroField label="Image">
            <span className="font-mono">{app.image}</span>
          </HeroField>
          <HeroField label="Node">
            <span className="font-mono">
              {app.node_id || (
                <span className="text-muted-foreground italic">
                  local control plane
                </span>
              )}
            </span>
          </HeroField>
        </div>

        <div className="border-t border-border pt-4">
          <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
            Setup checklist
          </h3>
          <ul className="mt-3 space-y-2.5">
            <ChecklistRow
              met={Boolean(app.node_id)}
              severity="neutral"
              label="Node assigned"
              detail={
                app.node_id
                  ? `Running on ${app.node_id}.`
                  : 'No node explicitly assigned; running on the local control plane by default.'
              }
              appName={app.name}
            />
            <ChecklistRow
              met={primaryDomain !== null}
              severity="warning"
              label="Domain connected"
              detail={
                primaryDomain
                  ? `Serving ${primaryDomain}.`
                  : network?.running && network.host_port
                    ? `No domain connected yet. Reachable right now at this server's address on port ${network.host_port}.`
                    : 'No domain connected yet, and the container is not currently running.'
              }
              action={
                primaryDomain
                  ? undefined
                  : { label: 'Connect a domain', to: '/apps/$name/domains' }
              }
              extraAction={
                primaryDomain || !tunnelAvailable
                  ? undefined
                  : {
                      label: 'Open Cloudflare Tunnel settings',
                      detail:
                        'A public URL is available via the connected Cloudflare Tunnel.',
                      to: '/settings/cloudflare-tunnel',
                    }
              }
              appName={app.name}
            />
            <ChecklistRow
              met={hasHealthCheck}
              severity="warning"
              label="Health check configured"
              detail={
                hasHealthCheck
                  ? 'A readiness or liveness probe is configured.'
                  : 'No readiness or liveness probe configured yet.'
              }
              action={
                hasHealthCheck
                  ? undefined
                  : { label: 'Configure health checks', to: '/apps/$name/health' }
              }
              appName={app.name}
            />
            <ChecklistRow
              met={hasResourceLimits}
              severity="neutral"
              label="Resource limits set"
              detail={
                hasResourceLimits
                  ? `Memory ${formatBytes(app.resources?.memory_bytes)}, CPU ${formatNanoCpus(app.resources?.nano_cpus)}.`
                  : 'No memory or CPU limit set, a valid choice, not a problem, worth revisiting if usage needs to stay predictable.'
              }
              action={
                hasResourceLimits
                  ? undefined
                  : { label: 'Set resource limits', to: '/apps/$name/resources' }
              }
              appName={app.name}
            />
          </ul>
        </div>
      </CardContent>
    </Card>
  )
}

function HeroField({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="min-w-0">
      <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {label}
      </dt>
      <dd className="mt-0.5 flex min-w-0 items-center text-sm text-foreground">
        {children}
      </dd>
    </div>
  )
}

// severity only governs styling for the unmet case: a met item always
// renders as a green check regardless of which severity its unmet state
// would have used. "warning" (amber) is for items that affect whether
// the app is actually reachable/observable (domain, health check);
// "neutral" (muted info icon) is for items that are legitimate to leave
// unset (node placement defaults sensibly to the local control plane,
// resource limits are opt-in by design, see internal/store.DesiredService's
// own NodeID doc comment and this repo's rules on hardcoded thresholds).
const SEVERITY_ICON: Record<'warning' | 'neutral', Icon> = {
  warning: WarningCircleIcon,
  neutral: InfoIcon,
}
const SEVERITY_ICON_CLASS: Record<'warning' | 'neutral', string> = {
  warning: 'text-amber-600 dark:text-amber-400',
  neutral: 'text-muted-foreground',
}

function ChecklistRow({
  met,
  severity,
  label,
  detail,
  action,
  extraAction,
  appName,
}: {
  met: boolean
  severity: 'warning' | 'neutral'
  label: string
  detail: string
  action?: { label: string; to: '/apps/$name/domains' | '/apps/$name/health' | '/apps/$name/resources' }
  // A second, distinct CTA for an unmet checklist item, scoped to
  // routes with no {name} param (unlike action above): today only the
  // "Domain connected" row's Cloudflare Tunnel pointer uses this, kept
  // as its own prop rather than a union with action so the two never
  // get confused about which params shape they need.
  extraAction?: { label: string; detail: string; to: '/settings/cloudflare-tunnel' }
  appName: string
}) {
  const Icon = met ? CheckCircleIcon : SEVERITY_ICON[severity]
  const iconClass = met
    ? 'text-green-600 dark:text-green-400'
    : SEVERITY_ICON_CLASS[severity]

  return (
    <li className="flex items-start gap-2.5 text-sm">
      <Icon className={`mt-0.5 size-4 shrink-0 ${iconClass}`} aria-hidden="true" />
      <div className="min-w-0 flex-1">
        <p className="font-medium text-foreground">{label}</p>
        <p className="text-muted-foreground">{detail}</p>
        {action ? (
          <Link
            to={action.to}
            params={{ name: appName }}
            className="mt-0.5 inline-block text-xs text-primary underline underline-offset-2"
          >
            {action.label}
          </Link>
        ) : null}
        {extraAction ? (
          <div className="mt-1.5 border-t border-border/60 pt-1.5">
            <p className="text-muted-foreground">{extraAction.detail}</p>
            <Link
              to={extraAction.to}
              className="mt-0.5 inline-block text-xs text-primary underline underline-offset-2"
            >
              {extraAction.label}
            </Link>
          </div>
        ) : null}
      </div>
    </li>
  )
}
