import { createFileRoute } from '@tanstack/react-router'
import { IntegrationsPlaceholder } from '../../../components/databases/home/IntegrationsPlaceholder'

export const Route = createFileRoute('/databases/$name/integrations')({
  component: IntegrationsPlaceholder,
})
