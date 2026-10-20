import {
  ArrowRightIcon,
  InfoIcon,
  LockIcon,
} from '@phosphor-icons/react/dist/ssr'
import type {
  EnvironmentCompare,
  EnvironmentEnvDiffEntry,
  EnvironmentEnvDiffStatus,
} from '../types/environmentCompare'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'

// Human labels/variants for environmentEnvDiffEntry.status
// (internal/api/environment_compare.go), the same "one map per field"
// convention DeployCompareView.tsx's own CHANGE_FIELD_LABEL/
// ENV_CHANGE_STATUS_LABEL already establish for its sibling feature.
const DIFF_STATUS_LABEL: Record<EnvironmentEnvDiffStatus, string> = {
  only_in_a: 'Only in A',
  only_in_b: 'Only in B',
  changed: 'Changed',
  masked: 'Secret (masked)',
}

const DIFF_STATUS_VARIANT: Record<
  EnvironmentEnvDiffStatus,
  'success' | 'destructive' | 'warning' | 'outline'
> = {
  only_in_a: 'success',
  only_in_b: 'destructive',
  changed: 'warning',
  masked: 'outline',
}

// EnvironmentEnvCompareView renders GET .../environments/compare's
// drift report: which env var keys differ between two of a project's
// environments, visually following DeployCompareView.tsx's own
// diff-table pattern (DeployCompareEnvChangesCard) for consistency,
// rather than inventing a new look for this sibling feature. A
// secret-marked key never shows a value, on either side: see
// internal/api/environment_compare.go's own doc comment for exactly
// when such a key still appears (added/removed/masked) versus not at
// all (same value, both plain).
export function EnvironmentEnvCompareView({
  compare,
}: {
  compare: EnvironmentCompare
}) {
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Env var drift</CardTitle>
        </CardHeader>
        <CardContent>
          {compare.diff.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No env var keys differ between {compare.a.environment.name} and{' '}
              {compare.b.environment.name}.
            </p>
          ) : (
            <ul className="space-y-2">
              {compare.diff.map((d) => (
                <EnvironmentEnvDiffRow key={d.key} entry={d} />
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Alert>
        <InfoIcon className="size-4" />
        <AlertDescription>{compare.note}</AlertDescription>
      </Alert>
    </div>
  )
}

function EnvironmentEnvDiffRow({ entry }: { entry: EnvironmentEnvDiffEntry }) {
  return (
    <li className="flex flex-wrap items-center gap-2 text-sm">
      <span className="flex w-40 shrink-0 items-center gap-1 font-mono text-xs font-medium text-foreground">
        {entry.secret ? (
          <LockIcon
            className="size-3.5 shrink-0 text-muted-foreground"
            aria-label="Secret value"
          />
        ) : null}
        <span className="truncate">{entry.key}</span>
      </span>
      <Badge variant={DIFF_STATUS_VARIANT[entry.status]}>
        {DIFF_STATUS_LABEL[entry.status]}
      </Badge>
      {entry.secret ? (
        <span className="text-xs text-muted-foreground/70">
          value not shown
        </span>
      ) : (
        <>
          <span className="truncate font-mono text-xs text-muted-foreground">
            {entry.a || '(none)'}
          </span>
          <ArrowRightIcon
            className="size-3.5 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <span className="truncate font-mono text-xs text-foreground">
            {entry.b || '(none)'}
          </span>
        </>
      )}
    </li>
  )
}
