import { createFileRoute } from '@tanstack/react-router'
import { FunctionsPage } from '../components/functions/FunctionsPage'

export const Route = createFileRoute('/functions')({
  component: FunctionsPage,
})
