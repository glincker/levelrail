import type { AppDetail, ServiceProbe } from '../types/appDetail'
import { formatBytes, formatDurationNs, formatNanoCpus } from '../lib/format'

// Read-only display of an app's current desired state: image, port,
// resource limits, and health probe config, straight from GET
// /api/v1/apps/{name}. Domains and env are editable elsewhere
// (DomainEditor, EnvEditor); everything shown here has no edit affordance
// on this pass, matching TASKS.md 1.10's scope for this route.
export function AppOverview({ app }: { app: AppDetail }) {
  return (
    <section className="rounded-lg border border-neutral-200 p-4 dark:border-neutral-800">
      <dl className="grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2">
        <Field label="Image" value={app.image} mono />
        <Field label="Port" value={String(app.port)} />
        <Field
          label="Memory limit"
          value={formatBytes(app.resources?.memory_bytes)}
        />
        <Field
          label="CPU limit"
          value={formatNanoCpus(app.resources?.nano_cpus)}
        />
      </dl>
      <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
        <ProbeSummary label="Readiness probe" probe={app.health?.readiness} />
        <ProbeSummary label="Liveness probe" probe={app.health?.liveness} />
      </div>
    </section>
  )
}

function Field({
  label,
  value,
  mono,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div>
      <dt className="text-xs font-medium tracking-wide text-neutral-500 uppercase dark:text-neutral-400">
        {label}
      </dt>
      <dd
        className={`mt-0.5 text-sm text-neutral-900 dark:text-neutral-100 ${mono ? 'font-mono' : ''}`}
      >
        {value}
      </dd>
    </div>
  )
}

function ProbeSummary({
  label,
  probe,
}: {
  label: string
  probe?: ServiceProbe | null
}) {
  return (
    <div className="rounded-md bg-neutral-50 p-3 dark:bg-neutral-900">
      <p className="text-xs font-medium tracking-wide text-neutral-500 uppercase dark:text-neutral-400">
        {label}
      </p>
      {probe ? (
        <ul className="mt-1 space-y-0.5 text-sm text-neutral-700 dark:text-neutral-300">
          <li>
            Path: <span className="font-mono">{probe.path}</span>
          </li>
          <li>Interval: {formatDurationNs(probe.interval)}</li>
          <li>Timeout: {formatDurationNs(probe.timeout)}</li>
          <li>Failure threshold: {probe.failures ?? 'not set'}</li>
        </ul>
      ) : (
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          not configured
        </p>
      )}
    </div>
  )
}
