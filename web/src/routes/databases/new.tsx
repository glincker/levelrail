import { createFileRoute, redirect } from '@tanstack/react-router'

// "New database" has no page of its own: the create flow is a dialog on the
// list, and without this file /databases/new matched $name as a database.
export const Route = createFileRoute('/databases/new')({
  beforeLoad: () => {
    // eslint-disable-next-line @typescript-eslint/only-throw-error
    throw redirect({ to: '/databases', search: { create: '1' }, replace: true })
  },
})
