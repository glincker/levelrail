import { WarningCircleIcon } from '@phosphor-icons/react/dist/ssr'
import type { PipelineRunReport } from '../types/pipelines'

const PROVIDER_LABEL: Record<string, string> = {
  github: 'GitHub',
  gitlab: 'GitLab',
  gitea: 'Gitea',
  bitbucket: 'Bitbucket',
}

// PipelineReportLink says where a run's commit status went: a link to the
// commit on the forge, or the reason the post failed. A failed post never
// changes the run's own outcome.
export function PipelineReportLink({ report }: { report?: PipelineRunReport }) {
  if (!report || (!report.state && !report.warning)) {
    return <span className="text-muted-foreground">-</span>
  }
  const provider = PROVIDER_LABEL[report.provider ?? ''] ?? 'the git provider'
  if (report.warning) {
    return (
      <span
        className="inline-flex items-center gap-1 text-amber-700 dark:text-amber-400"
        title={report.warning}
      >
        <WarningCircleIcon className="size-3.5" aria-hidden="true" />
        Not reported to {provider}
        <span className="sr-only">: {report.warning}</span>
      </span>
    )
  }
  const label = `Reported to ${provider}`
  if (!report.url) {
    return <span className="text-muted-foreground">{label}</span>
  }
  return (
    <a
      href={report.url}
      target="_blank"
      rel="noreferrer noopener"
      className="underline underline-offset-2"
    >
      {label} ({report.state})
    </a>
  )
}
