import { createFileRoute, Outlet } from '@tanstack/react-router'

// Thin pass-through layout, required only so $id/environments/$envId.tsx
// (a full standalone page, not a section sharing this one's chrome) has
// a registered parent route to attach to in TanStack Router's directory
// convention. The actual project detail page lives at $id/index.tsx.
export const Route = createFileRoute('/projects/$id')({
  component: Outlet,
})
