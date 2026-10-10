import { createFileRoute } from '@tanstack/react-router'
import { DatabaseExplorerPanel } from '../../../components/dbviewer/DatabaseExplorerPanel'

export const Route = createFileRoute('/databases/$name/explorer')({
  component: ExplorerSection,
})

function ExplorerSection() {
  const { name } = Route.useParams()
  return <DatabaseExplorerPanel databaseName={name} />
}
