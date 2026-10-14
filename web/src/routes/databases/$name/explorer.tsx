import { createFileRoute } from '@tanstack/react-router'
import { DatabaseExplorerPanel } from '../../../components/dbviewer/DatabaseExplorerPanel'
import { DemoGate } from '../../../components/demo/DemoUnavailable'

export const Route = createFileRoute('/databases/$name/explorer')({
  component: ExplorerSection,
})

function ExplorerSection() {
  const { name } = Route.useParams()
  return (
    <DemoGate reason="database">
      <DatabaseExplorerPanel databaseName={name} />
    </DemoGate>
  )
}
