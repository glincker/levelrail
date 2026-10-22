import { createFileRoute } from '@tanstack/react-router'
import { BenchmarksPage } from '../../components/settings/BenchmarksPage'

export const Route = createFileRoute('/settings/benchmarks')({
  component: BenchmarksPage,
})
