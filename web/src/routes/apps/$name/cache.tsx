import { createFileRoute } from '@tanstack/react-router'
import { AppCachePanel } from '../../../components/cdn/AppCachePanel'

// App-scoped response cache view (internal/api/cache_app.go).
export const Route = createFileRoute('/apps/$name/cache')({
  component: CacheSection,
})

function CacheSection() {
  const { name } = Route.useParams()
  return <AppCachePanel app={name} />
}
