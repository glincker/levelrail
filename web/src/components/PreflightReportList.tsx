import {
  CheckCircleIcon,
  WarningCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { PreflightReport, PreflightStatus } from '../types/preflight'

const STATUS_LABEL: Record<PreflightStatus, string> = {
  pass: 'Passed',
  warn: 'Warning',
  fail: 'Failed',
}

function StatusIcon({ status }: { status: PreflightStatus }) {
  if (status === 'pass') {
    return (
      <CheckCircleIcon className="size-4 text-success" aria-hidden="true" />
    )
  }
  if (status === 'warn') {
    return (
      <WarningCircleIcon className="size-4 text-warning" aria-hidden="true" />
    )
  }
  return <XCircleIcon className="size-4 text-destructive" aria-hidden="true" />
}

// Renders one preflight run: each check with its outcome, reason and fix
// hint. Purely presentational so the app overview and the create flow share it.
export function PreflightReportList({ report }: { report: PreflightReport }) {
  if (report.checks.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        Nothing to check yet. Fill in more of the form and run it again.
      </p>
    )
  }
  return (
    <ul className="space-y-2" aria-label="Preflight results">
      {report.checks.map((c) => (
        <li key={c.id} className="flex items-start gap-2 text-sm">
          <span className="mt-0.5">
            <StatusIcon status={c.status} />
            <span className="sr-only">{STATUS_LABEL[c.status]}</span>
          </span>
          <div>
            <p className="font-medium text-foreground">{c.name}</p>
            <p className="text-muted-foreground">{c.reason}</p>
            {c.fix && c.status !== 'pass' ? (
              <p className="text-xs text-foreground">{c.fix}</p>
            ) : null}
          </div>
        </li>
      ))}
    </ul>
  )
}
