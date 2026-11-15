// Multi-file templates become one multi-document YAML so the editor stays a single buffer.
export function joinTemplateFiles(files: { content: string }[]): string {
  return files.map((f) => f.content.trimEnd()).join('\n---\n') + '\n'
}

export type LocalIacTemplateId = 'cron-job'

export interface LocalIacTemplate {
  id: LocalIacTemplateId
  kinds: string[]
  yaml: string
}

// Starters that ship with the dashboard, so they work before any stack template exists.
export const LOCAL_IAC_TEMPLATES: LocalIacTemplate[] = [
  {
    id: 'cron-job',
    kinds: ['CronJob'],
    yaml: `version: 1
kind: CronJob
metadata:
  name: nightly-report
spec:
  app: web
  schedule: "0 3 * * *"
  timezone: UTC
  command: ["node", "scripts/report.js"]
  # Or call a path on the app instead of a command:
  # http:
  #   path: /internal/report
  #   method: POST
  #   headers:
  #     - name: Authorization
  #       secretRef: CRON_TOKEN
  #       prefix: Bearer
`,
  },
]
