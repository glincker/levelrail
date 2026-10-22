import { createFileRoute } from '@tanstack/react-router'
import { ControlPlaneHealthPage } from '../../components/settings/ControlPlaneHealthPage'

export const Route = createFileRoute('/settings/control-plane-health')({
  component: ControlPlaneHealthPage,
})
