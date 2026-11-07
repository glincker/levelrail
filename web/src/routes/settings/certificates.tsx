import { createFileRoute, redirect } from '@tanstack/react-router'

// Certificates now live in the Domains area; old links keep working.
export const Route = createFileRoute('/settings/certificates')({
  beforeLoad: () => {
    redirect({ to: '/domains/certificates', replace: true, throw: true })
  },
})
