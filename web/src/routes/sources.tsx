import { createFileRoute } from '@tanstack/react-router'
import { SourcesHub } from '../components/hubs/HubsPages'

export const Route = createFileRoute('/sources')({
  component: SourcesHub,
})
