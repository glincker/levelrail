import { createFileRoute } from '@tanstack/react-router'
import { ExecPanel } from '../../../components/ExecPanel'

// One-off command runner tab, the UI counterpart to POST
// /api/v1/apps/{name}/exec (internal/api/exec.go's handleExecApp). Only
// needs the app's name, not its full detail (ExecPanel doesn't render
// or edit any app config field), unlike most sibling tabs here that
// call useApp for the full record.
export const Route = createFileRoute('/apps/$name/exec')({
  component: ExecSection,
})

function ExecSection() {
  const { name } = Route.useParams()
  return <ExecPanel name={name} />
}
