import { createFileRoute, redirect } from '@tanstack/react-router'

// Old tab URL: the content now lives in the Settings shell.
export const Route = createFileRoute('/databases/$name/network')({
  beforeLoad: ({ params }) => {
    redirect({
      to: '/databases/$name/settings',
      params,
      search: { section: 'networking' },
      throw: true,
    })
  },
})
