import { createFileRoute } from '@tanstack/react-router'
import { AppFormsPanel } from '../../../components/forms/AppFormsPanel'

// App-scoped form capture (internal/api/forms_admin.go).
export const Route = createFileRoute('/apps/$name/forms')({
  component: FormsSection,
})

function FormsSection() {
  const { name } = Route.useParams()
  return <AppFormsPanel app={name} />
}
