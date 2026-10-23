import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  Link,
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router'
import { describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '@/test/commonI18n'
import { EnvVarsForm } from './EnvVarsForm'

type SaveFn = (vars: Record<string, string>, onSaved: () => void) => void

async function mount(onSave: SaveFn) {
  const root = createRootRoute()
  const index = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => (
      <div>
        <EnvVarsForm
          title="Environment variables"
          description="desc"
          emptyMessage="none"
          pastePlaceholder="KEY=value"
          values={{ FOO: 'bar' }}
          isPending={false}
          onSave={onSave}
        />
        <Link to="/login">leave</Link>
      </div>
    ),
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
  render(
    <CommonI18n>
      <RouterProvider router={router} />
    </CommonI18n>,
  )
  await screen.findByText('leave')
}

describe('EnvVarsForm unsaved guard', () => {
  it('guards an edit made after a successful save', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn(
      (_vars: Record<string, string>, onSaved: () => void) => {
        onSaved()
      },
    )
    await mount(onSave)
    const value = screen.getAllByRole('textbox')[1]!
    await user.type(value, 'x')
    await user.click(screen.getByRole('button', { name: 'Save variables' }))
    expect(onSave).toHaveBeenCalledWith({ FOO: 'barx' }, expect.any(Function))

    await user.type(screen.getAllByRole('textbox')[1]!, 'y')
    fireEvent.click(screen.getByText('leave'))
    expect(
      await screen.findByText('Discard unsaved changes?'),
    ).toBeInTheDocument()
    expect(screen.queryByText('other page')).not.toBeInTheDocument()
  })
})
