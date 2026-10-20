import { createFileRoute } from '@tanstack/react-router'
import { SplitTestingPanel } from '../../../components/SplitTestingPanel'

export const Route = createFileRoute('/apps/$name/split-testing')({
  component: SplitTestingSection,
})

function SplitTestingSection() {
  const { name } = Route.useParams()
  return <SplitTestingPanel appName={name} />
}
