import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ProjectsNavItem } from './ProjectsNavTree'
import { SidebarMenu, SidebarProvider } from '@/components/ui/sidebar'
import type { AppListEntry } from '@/types/appDetail'
import type { ProjectResource } from '@/types/projectDetail'
import type { GlobalNavItem } from './navModel'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      params,
      ...rest
    }: {
      children?: ReactNode
      to?: string
      params?: Record<string, string>
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => {
      const href = params
        ? Object.entries(params).reduce(
            (acc, [key, value]) => acc.replace(`$${key}`, value),
            to ?? '',
          )
        : to
      return (
        <a href={href} {...rest}>
          {children}
        </a>
      )
    },
  }
})

const mockProjects = vi.fn<() => { data: ProjectResource[] }>()
const mockApps =
  vi.fn<() => { data: Pick<AppListEntry, 'name' | 'project_id'>[] }>()

vi.mock('@/queries/projects', () => ({
  useProjectListOptional: () => mockProjects(),
}))

vi.mock('@/queries/apps', () => ({
  useAppListOptional: () => mockApps(),
}))

const projectsItem: GlobalNavItem = {
  id: 'projects',
  label: 'Projects',
  to: '/projects',
  icon: <span data-testid="projects-icon" />,
}

function renderTree(pathname = '/projects') {
  return render(
    <SidebarProvider>
      <SidebarMenu>
        <ProjectsNavItem item={projectsItem} pathname={pathname} />
      </SidebarMenu>
    </SidebarProvider>,
  )
}

describe('ProjectsNavItem', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.stubGlobal(
      'matchMedia',
      vi.fn().mockReturnValue({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    mockProjects.mockReset()
    mockApps.mockReset()
  })

  it('still links to /projects when no projects exist, with no expand affordance', () => {
    mockProjects.mockReturnValue({ data: [] })
    mockApps.mockReturnValue({ data: [] })
    renderTree()
    expect(screen.getByRole('link', { name: 'Projects' })).toHaveAttribute(
      'href',
      '/projects',
    )
    expect(screen.queryByRole('button', { name: /projects/i })).toBeNull()
  })

  it('renders real project names and expands to show assigned apps', async () => {
    mockProjects.mockReturnValue({
      data: [
        { id: 'proj-1', name: 'Storefront', created_at: '' },
        { id: 'proj-2', name: 'Internal Tools', created_at: '' },
      ],
    })
    mockApps.mockReturnValue({
      data: [
        { name: 'web', project_id: 'proj-1' },
        { name: 'worker', project_id: 'proj-1' },
        { name: 'standalone-app' },
      ],
    })
    const user = userEvent.setup()
    renderTree()

    expect(screen.getByText('Storefront')).toBeInTheDocument()
    expect(screen.getByText('Internal Tools')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /expand projects/i }))
    await user.click(screen.getByRole('button', { name: /expand storefront/i }))

    expect(screen.getByRole('link', { name: 'web' })).toHaveAttribute(
      'href',
      '/apps/web',
    )
    expect(screen.getByRole('link', { name: 'worker' })).toHaveAttribute(
      'href',
      '/apps/worker',
    )
    expect(screen.queryByText('standalone-app')).not.toBeInTheDocument()
  })

  it('shows a quiet "No apps yet" row for a project with no assigned apps, not an empty tree', async () => {
    mockProjects.mockReturnValue({
      data: [{ id: 'proj-empty', name: 'Empty Project', created_at: '' }],
    })
    mockApps.mockReturnValue({ data: [{ name: 'unrelated-app' }] })
    const user = userEvent.setup()
    renderTree()

    await user.click(screen.getByRole('button', { name: /expand projects/i }))
    await user.click(
      screen.getByRole('button', { name: /expand empty project/i }),
    )

    expect(screen.getByText('No apps yet')).toBeInTheDocument()
    expect(
      within(screen.getByText('No apps yet').closest('li')!).queryByRole(
        'link',
      ),
    ).toBeNull()
  })
})
