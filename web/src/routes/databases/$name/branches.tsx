import { createFileRoute } from '@tanstack/react-router'
import { BranchesPage } from '../../../components/databases/home/BranchesPage'

export const Route = createFileRoute('/databases/$name/branches')({
  component: BranchesSection,
})

function BranchesSection() {
  const { name } = Route.useParams()
  return <BranchesPage name={name} />
}
