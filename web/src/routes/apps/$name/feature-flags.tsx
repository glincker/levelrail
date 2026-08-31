import { createFileRoute } from '@tanstack/react-router'
import { FeatureFlagsPanel } from '../../../components/FeatureFlagsPanel'

// Real deep-linkable nested route, matching scheduled-tasks.tsx's own
// shape: routes/apps/$name.tsx's layout owns the loader/header, this
// file only renders the section itself.
export const Route = createFileRoute('/apps/$name/feature-flags')({
  component: FeatureFlagsSection,
})

function FeatureFlagsSection() {
  const { name } = Route.useParams()
  return <FeatureFlagsPanel appName={name} />
}
