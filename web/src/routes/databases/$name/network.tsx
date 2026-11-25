import { createFileRoute, redirect } from '@tanstack/react-router'

// Old tab URL: Networking now has its own page.
export const Route = createFileRoute('/databases/$name/network')({
  beforeLoad: ({ params }) => {
    redirect({
      to: '/databases/$name/networking',
      params,
      throw: true,
    })
  },
})
