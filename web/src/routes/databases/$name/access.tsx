import { createFileRoute } from '@tanstack/react-router'
import { AccessPanel } from '../../../components/databaseAccess/AccessPanel'

export const Route = createFileRoute('/databases/$name/access')({
  component: AccessSection,
})

function AccessSection() {
  const { name } = Route.useParams()
  return <AccessPanel databaseName={name} />
}
