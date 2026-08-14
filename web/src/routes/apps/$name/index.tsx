import { createFileRoute, redirect } from '@tanstack/react-router'

// Bare /apps/$name has no content of its own now that the 8 former tabs
// are real sibling routes: it redirects to overview, the same section the
// old client-side Tabs component defaulted to (`defaultValue="overview"`).
export const Route = createFileRoute('/apps/$name/')({
  beforeLoad: ({ params }) => {
    redirect({ to: '/apps/$name/overview', params, throw: true })
  },
})
