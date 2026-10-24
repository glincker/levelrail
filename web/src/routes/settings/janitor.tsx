import { createFileRoute, redirect } from '@tanstack/react-router'

// The Janitor page became the Disk and Docker tab of Cleanup; old links keep working.
export const Route = createFileRoute('/settings/janitor')({
  beforeLoad: () => {
    redirect({
      to: '/settings/cleanup',
      search: { tab: 'disk' },
      replace: true,
      throw: true,
    })
  },
})
