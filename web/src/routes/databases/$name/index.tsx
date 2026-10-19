import { createFileRoute, redirect } from '@tanstack/react-router'

// Bare /databases/$name redirects to overview; ?action= from the palette rides along.
export const Route = createFileRoute('/databases/$name/')({
  beforeLoad: ({ params, search }) => {
    redirect({
      to: '/databases/$name/overview',
      params,
      search: search.action ? { action: search.action } : {},
      throw: true,
    })
  },
})
