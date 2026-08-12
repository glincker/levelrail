import type { ConditionStatus, ReconcileCondition } from '../types/deploy'

const STATUS_STYLES: Record<ConditionStatus, string> = {
  True: 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300',
  False: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  Unknown:
    'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400',
}

// Renders the app's *current* reconcile status, not a deploy history
// log. GET /api/v1/apps/{name}/deploys (internal/api/deploys.go's
// handleDeployHistory) returns the latest condition per (controller,
// condition type) pair, since internal/store.UpsertConditions only ever
// keeps the newest row for each type; there is no attempt-by-attempt
// log to render. The heading and copy below are deliberately worded as
// "status", not "history", to match what the endpoint actually returns.
export function ConditionsPanel({
  conditions,
}: {
  conditions: ReconcileCondition[]
}) {
  if (conditions.length === 0) {
    return (
      <section className="rounded-lg border border-neutral-200 p-4 text-sm text-neutral-500 dark:border-neutral-800 dark:text-neutral-400">
        No reconcile status recorded yet.
      </section>
    )
  }

  return (
    <section className="rounded-lg border border-neutral-200 dark:border-neutral-800">
      <h2 className="border-b border-neutral-200 px-4 py-2 text-sm font-semibold text-neutral-900 dark:border-neutral-800 dark:text-neutral-100">
        Reconcile status
      </h2>
      <ul>
        {conditions.map((c) => (
          <li
            key={c.Type}
            className="flex items-start gap-3 border-b border-neutral-100 px-4 py-3 last:border-0 dark:border-neutral-900"
          >
            <span
              className={`mt-0.5 shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${STATUS_STYLES[c.Status]}`}
            >
              {c.Status}
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                {c.Type}: {c.Reason}
              </p>
              {c.Message ? (
                <p className="mt-0.5 text-sm text-neutral-500 dark:text-neutral-400">
                  {c.Message}
                </p>
              ) : null}
              <p className="mt-1 text-xs text-neutral-400 dark:text-neutral-500">
                Updated {new Date(c.LastTransitionTime).toLocaleString()}
              </p>
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}
