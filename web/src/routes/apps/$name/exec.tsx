import { createFileRoute } from '@tanstack/react-router'
import { AppTerminal } from '../../../components/AppTerminal'
import { ExecPanel } from '../../../components/ExecPanel'

// Two ways into the same container, in the order an operator reaches for
// them: an interactive shell (GET /api/v1/apps/{name}/terminal) for
// poking around, and the one-off command runner (POST
// /api/v1/apps/{name}/exec) for a single scripted check whose output you
// want to read rather than drive. Neither needs the app's full detail
// record, only its name.
export const Route = createFileRoute('/apps/$name/exec')({
  component: ExecSection,
})

function ExecSection() {
  const { name } = Route.useParams()
  return (
    <div className="space-y-4">
      <AppTerminal name={name} />
      <ExecPanel name={name} />
    </div>
  )
}
