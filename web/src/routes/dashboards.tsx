import { createFileRoute, redirect } from '@tanstack/react-router'

// The Dashboards page now lives on the home Dashboard; old links keep working.
export const Route = createFileRoute('/dashboards')({
  validateSearch: (
    search: Record<string, unknown>,
  ): { id?: string; board?: string } => ({
    ...(typeof search.id === 'string' && search.id ? { id: search.id } : {}),
    ...(typeof search.board === 'string' && search.board
      ? { board: search.board }
      : {}),
  }),
  beforeLoad: ({ search }) => {
    const board = search.board ?? search.id
    redirect({
      to: '/',
      search: board ? { board } : {},
      replace: true,
      throw: true,
    })
  },
})
