import { createFileRoute } from '@tanstack/react-router'
import { DatabaseConsolePanel } from '../../../components/dbviewer/DatabaseConsolePanel'
import { DemoGate } from '../../../components/demo/DemoUnavailable'

export const Route = createFileRoute('/databases/$name/console')({
  validateSearch: (search: Record<string, unknown>): { sql?: string } => ({
    sql: typeof search.sql === 'string' ? search.sql : undefined,
  }),
  component: ConsoleSection,
})

function ConsoleSection() {
  const { name } = Route.useParams()
  const { sql } = Route.useSearch()
  return (
    <DemoGate reason="database">
      <DatabaseConsolePanel databaseName={name} initialSql={sql} />
    </DemoGate>
  )
}
