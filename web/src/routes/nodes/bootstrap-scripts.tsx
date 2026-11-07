import { createFileRoute } from '@tanstack/react-router'
import { BootstrapScriptsPage } from '../../components/nodes/BootstrapScriptsPage'

export const Route = createFileRoute('/nodes/bootstrap-scripts')({
  component: BootstrapScriptsPage,
})
