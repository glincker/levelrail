import { createFileRoute, redirect } from '@tanstack/react-router'

// The app list is the landing route (frontend-plan.md section 1), so the
// bare root just redirects there. No component, no data fetching here.
// `throw: true` makes redirect() throw its own Response internally,
// rather than the call site doing `throw redirect(...)`: redirect()
// returns a Response, not an Error, so throwing it directly trips
// @typescript-eslint/only-throw-error.
export const Route = createFileRoute('/')({
  beforeLoad: () => {
    redirect({ to: '/apps', throw: true })
  },
})
