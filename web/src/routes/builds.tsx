import { createFileRoute } from '@tanstack/react-router'
import { BuildsPage } from '../components/builds/BuildsPage'

export const Route = createFileRoute('/builds')({
  component: BuildsPage,
})
