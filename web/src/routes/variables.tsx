import { createFileRoute } from '@tanstack/react-router'
import { VariablesHub } from '../components/hubs/HubsPages'

export const Route = createFileRoute('/variables')({
  component: VariablesHub,
})
