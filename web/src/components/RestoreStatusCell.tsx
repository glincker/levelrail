import { StatusBadge, type AttemptStatus } from './backupAttemptStatus'

export function RestoreStatusCell({
  status,
  error,
}: {
  status: AttemptStatus
  error?: string
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <StatusBadge status={status} />
      {status === 'failed' && error ? (
        <span
          className="max-w-[20rem] truncate text-xs text-destructive"
          title={error}
        >
          {error}
        </span>
      ) : null}
    </div>
  )
}
