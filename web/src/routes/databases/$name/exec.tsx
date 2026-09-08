import { createFileRoute } from '@tanstack/react-router'
import { ExecDatabasePanel } from '../../../components/ExecDatabasePanel'

// One-off command runner tab, the UI counterpart to POST
// /api/v1/databases/{name}/exec (internal/api/database_exec.go's
// handleExecDatabase). Mirrors routes/apps/$name/exec.tsx: only needs
// the database's name, not its full detail.
export const Route = createFileRoute('/databases/$name/exec')({
  component: ExecSection,
})

function ExecSection() {
  const { name } = Route.useParams()
  return <ExecDatabasePanel name={name} />
}
