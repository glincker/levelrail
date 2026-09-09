import { useState } from 'react'
import {
  ArrowRightIcon,
  ArrowCounterClockwiseIcon,
  InfoIcon,
  LockIcon,
  DatabaseIcon,
} from '@phosphor-icons/react/dist/ssr'
import type {
  DeployCompare,
  DeployCompareEnvChange,
  DeployCompareSide,
} from '../types/deployCompare'
import type { EnvironmentResource } from '../types/environment'
import { useApp } from '../queries/apps'
import { useTriggerDeploy } from '../queries/deploys'
import { useProtectedEnvironment } from '../queries/environments'
import { formatDeployDuration } from '../lib/deployDuration'
import { formatBytes, formatNanoCpus } from '../lib/format'
import { DEPLOY_ATTEMPT_SOURCE_LABEL } from '../lib/deployAttemptPresentation'
import type { DeployAttemptSource } from '../types/deployAttempt'
import { ProtectedEnvironmentNotice } from './ProtectedEnvironmentNotice'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'

// Human labels for deployCompareResource.Changes[].Field
// (internal/api/deploy_compare.go's diffDeployCompareSides), the wire
// vocabulary this component renders.
const CHANGE_FIELD_LABEL: Record<string, string> = {
  image: 'Image',
  commit_sha: 'Commit',
  source: 'Trigger source',
  port: 'Port',
  host_port: 'Host port',
  domains: 'Domains',
  'resources.memory_bytes': 'Memory limit',
  'resources.nano_cpus': 'CPU limit',
  'resources.swap_memory_bytes': 'Swap limit',
  'resources.cpuset_cpus': 'CPU set',
  strategy: 'Deploy strategy',
  replicas: 'Replicas',
  volumes: 'Volumes',
  labels: 'Labels',
  'health.readiness.path': 'Readiness path',
  'health.readiness.interval': 'Readiness interval',
  'health.readiness.timeout': 'Readiness timeout',
  'health.readiness.failures': 'Readiness failure threshold',
  'health.liveness.path': 'Liveness path',
  'health.liveness.interval': 'Liveness interval',
  'health.liveness.timeout': 'Liveness timeout',
  'health.liveness.failures': 'Liveness failure threshold',
}

// Human labels for deployCompareField.From/To when field is "strategy":
// the same three values DeployStrategyEditor.tsx's own STRATEGY_LABELS
// already maps, kept as a separate copy here since that map isn't
// exported and this view has no other reason to import that editor.
const STRATEGY_VALUE_LABEL: Record<string, string> = {
  recreate: 'Recreate',
  'blue-green': 'Blue-green',
  rolling: 'Rolling',
}

// Resource fields arrive as raw byte/nano-CPU counts stringified onto the
// wire (deployCompareField.From/To are both plain strings, the same shape
// image/commit_sha already use): reuse the app-editor's own formatters
// rather than showing an operator a raw byte count. Health interval/
// timeout fields arrive pre-formatted as Go duration strings (e.g. "5s"),
// already human-readable, so no further conversion is needed for those.
function formatChangeValue(field: string, value: string): string {
  if (!value) {
    return '(none)'
  }
  if (field === 'resources.memory_bytes' || field === 'resources.swap_memory_bytes') {
    return formatBytes(Number(value))
  }
  if (field === 'resources.nano_cpus') {
    return formatNanoCpus(Number(value))
  }
  if (field === 'strategy') {
    return STRATEGY_VALUE_LABEL[value] ?? value
  }
  return value
}

const ENV_CHANGE_STATUS_LABEL: Record<DeployCompareEnvChange['status'], string> = {
  added: 'Added',
  removed: 'Removed',
  changed: 'Changed',
}

const ENV_CHANGE_STATUS_VARIANT: Record<
  DeployCompareEnvChange['status'],
  'success' | 'destructive' | 'warning'
> = {
  added: 'success',
  removed: 'destructive',
  changed: 'warning',
}

// DeployCompareView renders GET .../deploys/compare's before/after diff:
// two side cards (each with its own "Roll back to this" CTA, distinct
// from the deploy history list's own rollback button, per this task's
// own requirement), the fields that actually differ, and an explicit
// note about what deploy_attempts never snapshotted rather than a
// fabricated diff for data that was never captured.
export function DeployCompareView({
  appName,
  compare,
}: {
  appName: string
  compare: DeployCompare
}) {
  const { data: app } = useApp(appName)
  const protectedEnv = useProtectedEnvironment(app)
  const [ackProtected, setAckProtected] = useState(false)

  return (
    <div className="space-y-4">
      {protectedEnv?.protected ? (
        <ProtectedEnvironmentNotice
          id="deploy-compare-ack-protected"
          environmentName={protectedEnv.name}
          acknowledged={ackProtected}
          onAcknowledgedChange={setAckProtected}
        />
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2">
        <DeployCompareSideCard
          appName={appName}
          label="From"
          side={compare.from}
          protectedEnv={protectedEnv}
          ackProtected={ackProtected}
        />
        <DeployCompareSideCard
          appName={appName}
          label="To"
          side={compare.to}
          protectedEnv={protectedEnv}
          ackProtected={ackProtected}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>What changed</CardTitle>
        </CardHeader>
        <CardContent>
          {compare.changes.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No tracked fields differ between these two deploys.
            </p>
          ) : (
            <ul className="space-y-2">
              {compare.changes.map((c) => (
                <li
                  key={c.field}
                  className="flex flex-wrap items-center gap-2 text-sm"
                >
                  <span className="w-32 shrink-0 font-medium text-foreground">
                    {CHANGE_FIELD_LABEL[c.field] ?? c.field}
                  </span>
                  <span className="truncate font-mono text-xs text-muted-foreground">
                    {formatChangeValue(c.field, c.from)}
                  </span>
                  <ArrowRightIcon
                    className="size-3.5 shrink-0 text-muted-foreground"
                    aria-hidden="true"
                  />
                  <span className="truncate font-mono text-xs text-foreground">
                    {formatChangeValue(c.field, c.to)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <DeployCompareEnvChangesCard changes={compare.env_changes ?? []} />

      <Alert>
        <InfoIcon className="size-4" />
        <AlertDescription>
          <p>{compare.note}</p>
          {compare.unsnapshotted_fields.length > 0 ? (
            <p className="mt-1.5 font-mono text-xs opacity-80">
              Not tracked: {compare.unsnapshotted_fields.join(', ')}
            </p>
          ) : null}
        </AlertDescription>
      </Alert>
    </div>
  )
}

// DeployCompareEnvChangesCard renders compare.env_changes: env is
// key-based rather than a single scalar, so it gets its own list instead
// of folding into the "What changed" card above, but follows the same
// row shape (label, from, arrow, to). A secret- or database-backed key
// never shows a from/to value, only its key, kind, and whether it was
// added or removed: internal/api/deploy_compare.go's diffDeployCompareEnv
// never reports one present unchanged on both sides as "changed", since
// there is no way to know that without decrypting a secret or
// re-resolving a live database reference.
function DeployCompareEnvChangesCard({
  changes,
}: {
  changes: DeployCompareEnvChange[]
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Environment variable changes</CardTitle>
      </CardHeader>
      <CardContent>
        {changes.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No environment variable keys differ between these two deploys.
          </p>
        ) : (
          <ul className="space-y-2">
            {changes.map((c) => (
              <li
                key={c.key}
                className="flex flex-wrap items-center gap-2 text-sm"
              >
                <span className="flex w-40 shrink-0 items-center gap-1 font-mono text-xs font-medium text-foreground">
                  {c.kind === 'secret' ? (
                    <LockIcon
                      className="size-3.5 shrink-0 text-muted-foreground"
                      aria-label="Secret value"
                    />
                  ) : null}
                  {c.kind === 'database' ? (
                    <DatabaseIcon
                      className="size-3.5 shrink-0 text-muted-foreground"
                      aria-label="Database-resolved value"
                    />
                  ) : null}
                  <span className="truncate">{c.key}</span>
                </span>
                <Badge variant={ENV_CHANGE_STATUS_VARIANT[c.status]}>
                  {ENV_CHANGE_STATUS_LABEL[c.status]}
                </Badge>
                {c.kind === 'literal' ? (
                  <>
                    <span className="truncate font-mono text-xs text-muted-foreground">
                      {c.from || '(none)'}
                    </span>
                    <ArrowRightIcon
                      className="size-3.5 shrink-0 text-muted-foreground"
                      aria-hidden="true"
                    />
                    <span className="truncate font-mono text-xs text-foreground">
                      {c.to || '(none)'}
                    </span>
                  </>
                ) : (
                  <span className="text-xs text-muted-foreground/70">
                    value not shown
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

function DeployCompareSideCard({
  appName,
  label,
  side,
  protectedEnv,
  ackProtected,
}: {
  appName: string
  label: string
  side: DeployCompareSide
  protectedEnv: EnvironmentResource | undefined
  ackProtected: boolean
}) {
  const triggerDeploy = useTriggerDeploy(appName)

  const handleRollback = () => {
    triggerDeploy.mutate(
      { image: side.image, confirm: ackProtected },
      {
        onSuccess: () => {
          toast.add({
            title: 'Rollback triggered.',
            description: `Redeploying ${side.image}.`,
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle className="text-sm text-muted-foreground">
          {label}
        </CardTitle>
        {side.is_current ? (
          <Badge variant="outline">Currently running</Badge>
        ) : side.status ? (
          <Badge variant="outline">{side.status}</Badge>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-2">
        <p className="truncate font-mono text-sm font-medium text-foreground">
          {side.image}
        </p>
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {side.commit_sha ? (
            <span className="rounded bg-muted px-1.5 py-0.5 font-mono">
              {side.commit_sha.slice(0, 7)}
            </span>
          ) : null}
          {side.source ? (
            <Badge variant="outline" className="shrink-0">
              {DEPLOY_ATTEMPT_SOURCE_LABEL[side.source as DeployAttemptSource] ??
                side.source}
            </Badge>
          ) : null}
        </div>
        {side.started_at ? (
          <p className="text-xs text-muted-foreground/70">
            Started {new Date(side.started_at).toLocaleString()}
            {side.finished_at
              ? ` · ${formatDeployDuration(side.started_at, side.finished_at)}`
              : ''}
          </p>
        ) : null}

        {!side.is_current ? (
          <Button
            variant="outline"
            size="sm"
            className="mt-1"
            onClick={handleRollback}
            disabled={
              triggerDeploy.isPending ||
              (protectedEnv?.protected && !ackProtected)
            }
          >
            <ArrowCounterClockwiseIcon
              className="size-3.5"
              data-icon="inline-start"
            />
            {triggerDeploy.isPending ? 'Rolling back...' : 'Roll back to this build'}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  )
}
