import { createFileRoute } from '@tanstack/react-router'
import { BranchesPlaceholder } from '../../../components/databases/home/BranchesPlaceholder'

export const Route = createFileRoute('/databases/$name/branches')({
  component: BranchesPlaceholder,
})
