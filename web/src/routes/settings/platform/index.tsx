import { createFileRoute } from '@tanstack/react-router'
import { PlatformOverviewPage } from '../../../components/platform/PlatformOverviewPage'

export const Route = createFileRoute('/settings/platform/')({
  component: PlatformOverviewPage,
})
