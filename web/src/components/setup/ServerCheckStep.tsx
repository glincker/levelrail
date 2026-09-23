import { useQuery } from '@tanstack/react-query'
import {
  ArrowsClockwiseIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { DoctorCheckRow } from '../DoctorCheckRow'
import { systemDoctorQueryOptions } from '../../queries/systemDoctor'
import type { DoctorCheck } from '../../queries/systemDoctor'
import { hardFailures, serverCheckGate } from '../../lib/setupWizard'
import { StepFooter } from './StepChrome'
import type { StepProps } from './types'

const STATUS_ORDER: Record<DoctorCheck['status'], number> = {
  fail: 0,
  warn: 1,
  unknown: 2,
  ok: 3,
}

/** ServerCheckStep runs the doctor bundle and blocks only on checks nothing can deploy without. */
export function ServerCheckStep({ onContinue, pending }: StepProps) {
  const { data, isFetching, refetch, error } = useQuery(
    systemDoctorQueryOptions(),
  )
  const gate = serverCheckGate(data)
  const hard = data ? hardFailures(data) : []
  const sorted = data
    ? [...data.checks].sort(
        (a, b) => STATUS_ORDER[a.status] - STATUS_ORDER[b.status],
      )
    : []
  const warnings = sorted.filter(
    (c) => c.status === 'warn' || c.status === 'fail',
  ).length

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <p className="max-w-prose text-sm text-muted-foreground">
          Checks Docker, disk, ports, and whether the internet can reach this
          server. Warnings will not stop you, but fixing them now saves a failed
          deploy later.
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            void refetch()
          }}
          disabled={isFetching}
        >
          <ArrowsClockwiseIcon
            className={
              isFetching ? 'animate-spin motion-reduce:animate-none' : ''
            }
          />
          Re-run checks
        </Button>
      </div>

      {error ? (
        <Alert variant="destructive">
          <XCircleIcon />
          <AlertTitle>Could not run the checks</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : null}

      {hard.length > 0 ? (
        <Alert variant="destructive">
          <XCircleIcon />
          <AlertTitle>This server cannot run apps yet</AlertTitle>
          <AlertDescription>
            Fix the failed checks below, then re-run the checks.
          </AlertDescription>
        </Alert>
      ) : null}

      {data ? (
        <div>
          <p className="text-xs text-muted-foreground">
            {warnings === 0
              ? `No failures or warnings across ${sorted.length} checks.`
              : `${warnings} of ${sorted.length} checks need attention.`}
          </p>
          <div className="divide-y divide-border">
            {sorted.map((check) => (
              <DoctorCheckRow key={check.code} check={check} />
            ))}
          </div>
        </div>
      ) : (
        <ListSkeleton rows={5} />
      )}

      <StepFooter gate={gate} onContinue={onContinue} pending={pending} />
    </div>
  )
}
