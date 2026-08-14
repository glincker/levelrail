import { createFileRoute, redirect } from '@tanstack/react-router'

// Bare /databases/$name has no content of its own now that the layout
// route renders <Outlet /> for a real section route: redirects to
// overview, mirroring routes/apps/$name/index.tsx exactly (same reason:
// Databases has exactly one section today, so this always lands there).
export const Route = createFileRoute('/databases/$name/')({
  beforeLoad: ({ params }) => {
    redirect({ to: '/databases/$name/overview', params, throw: true })
  },
})
