import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import {
  HeartbeatIcon,
  CheckCircleIcon,
  WarningCircleIcon,
  XCircleIcon,
  MinusCircleIcon,
  ArrowsClockwiseIcon,
  StackIcon,
  ShieldCheckIcon,
  GlobeIcon,
  CopyIcon,
  CheckIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { Alert, AlertTitle, AlertDescription } from '@/components/ui/alert'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { HelpLink } from '@/components/HelpLink'
import {
  systemDoctorQueryOptions,
  useSystemDoctor,
} from '../../queries/systemDoctor'
import type { DoctorCheck, DoctorCheckStatus } from '../../queries/systemDoctor'
import { getCheckCta } from './system-status-ctas'

// Web half of "levelrail-cli doctor": that command already runs
// /api/v1/system/doctor's full preflight bundle (Docker reachability,
// disk space, data dir writable, ports 80/443, database, master key
// rotation age, ufw status) with no dashboard equivalent until this
// route. Dokploy's "Security Audit" page covers the same ground; this
// is that page here.
export const Route = createFileRoute('/settings/system-status')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(systemDoctorQueryOptions()),
  component: SystemStatusPage,
  pendingComponent: SystemStatusPending,
})

const STATUS_META: Record<
  DoctorCheckStatus,
  {
    label: string
    variant: VariantProps<typeof badgeVariants>['variant']
    icon: Icon
  }
> = {
  ok: { label: 'OK', variant: 'success', icon: CheckCircleIcon },
  warn: { label: 'Warning', variant: 'warning', icon: WarningCircleIcon },
  fail: { label: 'Failed', variant: 'destructive', icon: XCircleIcon },
  unknown: { label: 'Not checked', variant: 'muted', icon: MinusCircleIcon },
}

// Purely presentational grouping: the two categories an operator
// actually thinks in (can this box run containers at all, versus is it
// locked down), not a distinction the API itself makes. Any check code
// the backend adds later that isn't listed here still renders, just
// under "Other checks" rather than being silently dropped.
const INFRASTRUCTURE_CODES = [
  'docker',
  'disk_space',
  'data_dir_writable',
  'ram',
  'cpu',
]
const INFRASTRUCTURE_CODES_AFTER_PORTS = ['database']
const SECURITY_CODES = ['firewall', 'master_key_rotation', 'stale_secrets']
const NETWORK_CODES = ['public_ip', 'acme_reachability', 'clock_skew']

// PORT_CHECK_CODE matches port_<n> for whatever ports this instance's
// ingress is actually configured on (APP_INGRESS_HTTP_ADDR/
// APP_INGRESS_HTTPS_ADDR), not just the literal port_80/port_443 codes
// a default-port instance reports.
const PORT_CHECK_CODE = /^port_\d+$/
// EXTERNAL_REACHABILITY_CODE matches external_reachability_<n> the same
// way PORT_CHECK_CODE matches port_<n>, for whatever ports this
// instance's ingress is configured on.
const EXTERNAL_REACHABILITY_CODE = /^external_reachability_\d+$/

function groupChecks(checks: DoctorCheck[]) {
  const byCode = new Map(checks.map((c) => [c.code, c]))
  const take = (codes: string[]) =>
    codes
      .map((code) => byCode.get(code))
      .filter((c): c is DoctorCheck => Boolean(c))
  const portChecks = checks.filter((c) => PORT_CHECK_CODE.test(c.code))
  const externalReachabilityChecks = checks.filter((c) =>
    EXTERNAL_REACHABILITY_CODE.test(c.code),
  )
  const infrastructure = [
    ...take(INFRASTRUCTURE_CODES),
    ...portChecks,
    ...take(INFRASTRUCTURE_CODES_AFTER_PORTS),
  ]
  const security = take(SECURITY_CODES)
  const network = [
    ...take(['public_ip']),
    ...externalReachabilityChecks,
    ...take(['acme_reachability', 'clock_skew']),
  ]
  const seen = new Set([
    ...INFRASTRUCTURE_CODES,
    ...INFRASTRUCTURE_CODES_AFTER_PORTS,
    ...SECURITY_CODES,
    ...NETWORK_CODES,
    ...portChecks.map((c) => c.code),
    ...externalReachabilityChecks.map((c) => c.code),
  ])
  const other = checks.filter((c) => !seen.has(c.code))
  return { infrastructure, security, network, other }
}

// FixCommand renders a check's backend-supplied Fix as a copyable code
// block plus a HelpLink to its DocsPath, the CLI's own printSystemDoctorFixes
// (cmd/levelrail-cli/output.go) rendered as a table row instead: same
// two fields, different surface.
function FixCommand({ fix, docsPath }: { fix: string; docsPath?: string }) {
  const [copied, setCopied] = useState(false)

  function copyFix() {
    void navigator.clipboard.writeText(fix).then(() => {
      setCopied(true)
    })
  }

  return (
    <div className="mt-2 rounded-md bg-muted/50 p-2.5">
      <div className="flex items-start gap-2">
        <code className="min-w-0 flex-1 overflow-x-auto whitespace-pre-wrap break-all text-xs">
          {fix}
        </code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="shrink-0"
          onClick={copyFix}
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>
      {docsPath ? (
        <div className="mt-1.5">
          <HelpLink path={docsPath} label="Learn more" variant="inline" />
        </div>
      ) : null}
    </div>
  )
}

function CheckRow({ check }: { check: DoctorCheck }) {
  const meta = STATUS_META[check.status]
  const StatusIcon = meta.icon
  const cta = getCheckCta(check)
  return (
    <div className="py-2.5 text-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-medium text-foreground">{check.name}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {check.message}
          </p>
        </div>
        <Badge variant={meta.variant} className="shrink-0">
          <StatusIcon />
          {meta.label}
        </Badge>
      </div>
      {cta ? (
        <div className="mt-2 rounded-md bg-muted/50 p-2.5">
          <p className="text-xs text-muted-foreground">{cta.message}</p>
          <div className="mt-1.5">{cta.action}</div>
        </div>
      ) : null}
      {check.fix ? (
        <FixCommand fix={check.fix} docsPath={check.docs_path} />
      ) : null}
    </div>
  )
}

function CheckGroupCard({
  title,
  description,
  icon,
  checks,
}: {
  title: string
  description: string
  icon: Icon
  checks: DoctorCheck[]
}) {
  if (checks.length === 0) return null
  const GroupIcon = icon
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <GroupIcon className="size-4" />
          </div>
          <div>
            <CardTitle>{title}</CardTitle>
            <CardDescription>{description}</CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="divide-y divide-border">
        {checks.map((check) => (
          <CheckRow key={check.code} check={check} />
        ))}
      </CardContent>
    </Card>
  )
}

function SummaryBanner({ ok, checks }: { ok: boolean; checks: DoctorCheck[] }) {
  if (ok) {
    return (
      <Alert>
        <CheckCircleIcon />
        <AlertTitle>All checks passed</AlertTitle>
        <AlertDescription>
          Docker, disk space, ports, the database, and every other preflight
          check are all healthy right now.
        </AlertDescription>
      </Alert>
    )
  }
  const failed = checks.filter((c) => c.status === 'fail')
  return (
    <Alert variant="destructive">
      <XCircleIcon />
      <AlertTitle>One or more checks failed</AlertTitle>
      <AlertDescription>
        {failed.length > 0
          ? `Failing: ${failed.map((c) => c.name).join(', ')}.`
          : 'Review the checks below for details.'}
      </AlertDescription>
    </Alert>
  )
}

function SystemStatusPage() {
  const { data, isFetching, refetch } = useSystemDoctor()
  const { infrastructure, security, network, other } = groupChecks(data.checks)

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <HeartbeatIcon className="size-4" />
          </div>
          <div>
            <h1 className="flex items-center gap-1.5 text-lg font-semibold text-foreground">
              System status
              <HelpLink path="/troubleshooting" label="Troubleshooting guide" />
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Preflight checks: Docker, disk, ports, database, firewall, and
              outbound network reachability.
            </p>
          </div>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            void refetch()
          }}
          disabled={isFetching}
        >
          <ArrowsClockwiseIcon className={isFetching ? 'animate-spin' : ''} />
          Re-run checks
        </Button>
      </div>

      <SummaryBanner ok={data.ok} checks={data.checks} />

      <CheckGroupCard
        title="Infrastructure"
        description="Can this box run and reach what it needs to run containers."
        icon={StackIcon}
        checks={infrastructure}
      />
      <CheckGroupCard
        title="Security"
        description="Hardening checks: firewall status and secret rotation age."
        icon={ShieldCheckIcon}
        checks={security}
      />
      <CheckGroupCard
        title="Network and reachability"
        description="Outbound network access and whether the internet can reach this box."
        icon={GlobeIcon}
        checks={network}
      />
      <CheckGroupCard
        title="Other checks"
        description="Additional checks reported by this control plane."
        icon={HeartbeatIcon}
        checks={other}
      />
    </div>
  )
}

function SystemStatusPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">System status</h1>
      <ListSkeleton rows={5} />
    </div>
  )
}
