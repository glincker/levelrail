import type {
  IacChange,
  IacFieldChange,
  IacItemResult,
  IacPlan,
} from '../queries/iac'

const actionStyle: Record<string, string> = {
  create: 'text-green-700 dark:text-green-400',
  update: 'text-amber-700 dark:text-amber-400',
  delete: 'text-red-700 dark:text-red-400',
  error: 'text-red-700 dark:text-red-400',
  noop: 'text-muted-foreground',
}

const actionMark: Record<string, string> = {
  create: '+',
  update: '~',
  delete: '-',
  error: '!',
  noop: ' ',
}

function fieldText(f: IacFieldChange): string {
  switch (f.op) {
    case 'add':
      return `+ ${f.path}: ${f.new ?? ''}`
    case 'remove':
      return `- ${f.path}: ${f.old ?? ''}`
    case 'keep':
      return `= ${f.path} (kept, not in the files)`
    default:
      return `~ ${f.path}: ${f.old ?? ''} -> ${f.new ?? ''}`
  }
}

function changeKey(c: IacChange): string {
  return [c.kind, c.scope, c.name].filter(Boolean).join('/')
}

export function IacSummaryLine({ plan }: { plan: IacPlan }) {
  const s = plan.summary
  return (
    <p className="text-sm text-foreground">
      {s.create} to create, {s.update} to update, {s.delete} to delete, {s.noop}{' '}
      unchanged
      {s.error > 0 ? `, ${s.error} with errors` : ''}
    </p>
  )
}

function ChangeRow({ change }: { change: IacChange }) {
  return (
    <li className="rounded-md border border-border p-3">
      <div className="flex flex-wrap items-baseline gap-2 font-mono text-sm">
        <span className={actionStyle[change.action]}>
          {actionMark[change.action]}
        </span>
        <span className="text-foreground">{changeKey(change)}</span>
        {change.file ? (
          <span className="text-xs text-muted-foreground">
            {change.file}:{change.line}
          </span>
        ) : null}
        {change.denied ? (
          <span className="text-xs text-red-700 dark:text-red-400">denied</span>
        ) : null}
      </div>
      {change.reason ? (
        <p className="mt-1 text-sm text-red-700 dark:text-red-400">
          {change.reason}
        </p>
      ) : null}
      {change.fields?.length || change.kept?.length ? (
        <pre className="mt-2 overflow-x-auto font-mono text-xs text-muted-foreground">
          {[...(change.fields ?? []), ...(change.kept ?? [])]
            .map(fieldText)
            .join('\n')}
        </pre>
      ) : null}
      {change.warnings?.map((w) => (
        <p key={w} className="mt-1 text-xs text-amber-700 dark:text-amber-400">
          warning: {w}
        </p>
      ))}
    </li>
  )
}

export function IacPlanView({ plan }: { plan: IacPlan }) {
  const shown = plan.changes.filter((c) => c.action !== 'noop')
  return (
    <div className="space-y-3">
      <IacSummaryLine plan={plan} />
      {shown.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Live state already matches the files.
        </p>
      ) : (
        <ul className="space-y-2">
          {shown.map((c) => (
            <ChangeRow key={changeKey(c) + c.action} change={c} />
          ))}
        </ul>
      )}
    </div>
  )
}

const statusStyle: Record<string, string> = {
  applied: 'text-green-700 dark:text-green-400',
  failed: 'text-red-700 dark:text-red-400',
  denied: 'text-red-700 dark:text-red-400',
  skipped: 'text-amber-700 dark:text-amber-400',
  noop: 'text-muted-foreground',
}

export function IacResultView({ results }: { results: IacItemResult[] }) {
  const shown = results.filter((r) => r.status !== 'noop')
  return (
    <ul className="space-y-1.5">
      {shown.map((r) => (
        <li key={changeKey(r) + r.action} className="font-mono text-sm">
          <span className={statusStyle[r.status]}>{r.status}</span>{' '}
          <span className="text-foreground">{changeKey(r)}</span>
          {r.error ? (
            <span className="text-muted-foreground">: {r.error}</span>
          ) : null}
        </li>
      ))}
    </ul>
  )
}
