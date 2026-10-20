import { createFileRoute, redirect } from '@tanstack/react-router'

// Bare /apps/$name redirects to overview; ?action= from the palette rides along.
export const Route = createFileRoute('/apps/$name/')({
  beforeLoad: ({ params, search }) => {
    redirect({
      to: '/apps/$name/overview',
      params,
      search: search.action ? { action: search.action } : {},
      throw: true,
    })
  },
})
