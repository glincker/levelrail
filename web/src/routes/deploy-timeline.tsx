import { createFileRoute } from '@tanstack/react-router'
import { DeployTimelinePage } from '../components/deployTimeline/DeployTimelinePage'

export const Route = createFileRoute('/deploy-timeline')({
  component: DeployTimelinePage,
})
