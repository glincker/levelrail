import { useQuery } from '@tanstack/react-query'
import {
  ArrowsClockwiseIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { ServerCheckSummary } from './ServerCheckSummary'
import { systemDoctorQueryOptions } from '../../queries/systemDoctor'
import { serverCheckGate } from '../../lib/setupWizard'
import { StepFooter } from './StepChrome'
import type { StepProps } from './types'

/** ServerCheckStep runs the doctor bundle and blocks only on checks nothing can deploy without. */
export function ServerCheckStep({ onContinue, pending }: StepProps) {
  const { data, isFetching, refetch, error } = useQuery(
    systemDoctorQueryOptions(),
  )
  const gate = serverCheckGate(data)

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <p className="max-w-prose text-sm text-muted-foreground">
          We checked Docker, disk space, ports, and whether the internet can
          reach this server, so nothing surprises you later.
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

      {data ? <ServerCheckSummary report={data} /> : <ListSkeleton rows={5} />}

      <StepFooter gate={gate} onContinue={onContinue} pending={pending} />
    </div>
  )
}
