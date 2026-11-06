import type { ReactNode } from 'react'
import { renderHook } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { createTestQueryClient } from '@/test/testQueryClient'
import { userListQueryOptions, type UserResource } from '@/queries/users'
import type { Ability } from '@/types/token'
import { useAbilities } from './useAbilities'

vi.mock('./useAuthUsername', () => ({ useAuthUsername: () => 'me@x.io' }))

function user(email: string, abilities: Ability[]): UserResource {
  return { id: email, email, abilities } as UserResource
}

function run(users: UserResource[] | null) {
  const client = createTestQueryClient()
  if (users) client.setQueryData(userListQueryOptions().queryKey, users)
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  return renderHook(() => useAbilities(), { wrapper }).result.current
}

describe('useAbilities', () => {
  it('treats root as admin with every ability', () => {
    const a = run([user('me@x.io', ['root'])])
    expect(a.role).toBe('admin')
    expect(a.can('write:sensitive')).toBe(true)
    expect(a.can('root')).toBe(true)
  })

  it('maps the operator preset', () => {
    const a = run([
      user('me@x.io', ['read', 'read:sensitive', 'write', 'deploy']),
    ])
    expect(a.role).toBe('operator')
    expect(a.can('write')).toBe(true)
    expect(a.can('root')).toBe(false)
    expect(a.can('write:sensitive')).toBe(false)
  })

  it('maps the viewer preset', () => {
    const a = run([user('me@x.io', ['read'])])
    expect(a.role).toBe('viewer')
    expect(a.can('read')).toBe(true)
    expect(a.can('write')).toBe(false)
  })

  it('allows nothing while the user list is unknown', () => {
    const a = run(null)
    expect(a.role).toBeNull()
    expect(a.can('read')).toBe(false)
  })
})
