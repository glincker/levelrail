import { createFileRoute } from '@tanstack/react-router'
import { UpgradesPanel } from '../../../components/databaseUpgrades/UpgradesPanel'

export const Route = createFileRoute('/databases/$name/upgrades')({
  component: UpgradesSection,
})

function UpgradesSection() {
  const { name } = Route.useParams()
  return <UpgradesPanel databaseName={name} />
}
