import { createFileRoute, redirect } from '@tanstack/react-router'
import { fetchSession } from '../../queries/security'
import { setStoredUsername } from '../../lib/authStore'

// The OAuth callback redirects the browser here after setting the
// session cookie. This route bridges that into the client-side
// authStore heuristic before landing on the dashboard.
export const Route = createFileRoute('/oauth/complete')({
  beforeLoad: async () => {
    try {
      const session = await fetchSession()
      setStoredUsername(session.username)
    } catch {
      redirect({ to: '/login', throw: true })
      return
    }
    redirect({ to: '/apps', throw: true })
  },
  component: () => null,
})
