import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  Link,
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router'
import { CommonI18n } from '@/test/commonI18n'
import { useUnsavedGuard } from './useUnsavedGuard'

function Editor({ dirty }: Readonly<{ dirty: boolean }>) {
  const guard = useUnsavedGuard(dirty)
  return (
    <div>
      {guard}
      <Link to="/login">leave</Link>
    </div>
  )
}

async function mount(dirty: boolean) {
  const root = createRootRoute()
  const index = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => <Editor dirty={dirty} />,
  })
  const other = createRoute({
    getParentRoute: () => root,
    path: '/login',
    component: () => <p>other page</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([index, other]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  const view = render(
    <CommonI18n>
      <RouterProvider router={router} />
    </CommonI18n>,
  )
  await screen.findByText('leave')
  return view
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('useUnsavedGuard', () => {
  it('does not block navigation when clean', async () => {
    await mount(false)
    fireEvent.click(screen.getByText('leave'))
    expect(await screen.findByText('other page')).toBeInTheDocument()
  })

  it('blocks when dirty and keeps the user on Keep editing', async () => {
    await mount(true)
    fireEvent.click(screen.getByText('leave'))
    expect(
      await screen.findByText('Discard unsaved changes?'),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
    await waitFor(() => {
      expect(
        screen.queryByText('Discard unsaved changes?'),
      ).not.toBeInTheDocument()
    })
    expect(screen.queryByText('other page')).not.toBeInTheDocument()
  })

  it('proceeds when the user confirms Discard', async () => {
    await mount(true)
    fireEvent.click(screen.getByText('leave'))
    await screen.findByText('Discard unsaved changes?')
    fireEvent.click(screen.getByRole('button', { name: 'Discard' }))
    expect(await screen.findByText('other page')).toBeInTheDocument()
  })

  it('registers beforeunload only while dirty and removes it on unmount', async () => {
    const add = vi.spyOn(window, 'addEventListener')
    const remove = vi.spyOn(window, 'removeEventListener')
    const view = await mount(true)
    expect(add.mock.calls.some(([type]) => type === 'beforeunload')).toBe(true)
    act(() => {
      window.dispatchEvent(new Event('beforeunload', { cancelable: true }))
    })
    view.unmount()
    expect(remove.mock.calls.some(([type]) => type === 'beforeunload')).toBe(
      true,
    )
  })

  it('does not register beforeunload when clean', async () => {
    const add = vi.spyOn(window, 'addEventListener')
    await mount(false)
    expect(add.mock.calls.some(([type]) => type === 'beforeunload')).toBe(false)
  })
})
