import { createFileRoute } from '@tanstack/react-router'
import { ConsentCard } from '../../components/mcp/ConsentCard'

interface ConsentSearch {
  request?: string
}

// Plain function rather than zod so validateSearch stays out of the eager bundle.
function validateConsentSearch(search: Record<string, unknown>): ConsentSearch {
  const { request } = search
  return typeof request === 'string' && /^[\w-]{1,128}$/.test(request)
    ? { request }
    : {}
}

// Signed-out visitors are sent through /login by the root guard and return here.
export const Route = createFileRoute('/oauth/consent')({
  validateSearch: validateConsentSearch,
  component: ConsentPage,
})

function ConsentPage() {
  const { request } = Route.useSearch()
  return <ConsentCard requestId={request} />
}
