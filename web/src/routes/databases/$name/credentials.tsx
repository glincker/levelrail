import { createFileRoute } from '@tanstack/react-router'
import { CredentialsPanel } from '../../../components/databaseAccess/CredentialsPanel'

export const Route = createFileRoute('/databases/$name/credentials')({
  component: CredentialsSection,
})

function CredentialsSection() {
  const { name } = Route.useParams()
  return <CredentialsPanel databaseName={name} />
}
