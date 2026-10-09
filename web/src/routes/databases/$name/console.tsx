import { createFileRoute } from '@tanstack/react-router'
import { DatabaseConsolePanel } from '../../../components/dbviewer/DatabaseConsolePanel'

export const Route = createFileRoute('/databases/$name/console')({
  component: ConsoleSection,
})

function ConsoleSection() {
  const { name } = Route.useParams()
  return <DatabaseConsolePanel databaseName={name} />
}
