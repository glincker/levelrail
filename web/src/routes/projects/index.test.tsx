import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AppsEmptyActions } from '../../components/apps/AppsEmptyActions'
import { Route } from './index'

const projects = vi.hoisted(() => ({ list: [] as { id: string }[] }))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({ children, to }: { children?: ReactNode; to?: string }) => (
      <a href={to}>{children}</a>
    ),
  }
})
vi.mock('../../queries/projects', () => ({
  projectListQueryOptions: () => ({}),
  useProjects: () => ({ data: projects.list }),
}))
vi.mock('../../components/CreateProjectDialog', () => ({
  CreateProjectDialog: () => <button type="button">New project</button>,
}))
vi.mock('../../components/ProjectRow', () => ({
  PROJECT_LIST_GRID: '',
  ProjectRow: () => <div />,
  RowSkeleton: () => <div />,
}))

function renderProjects() {
  const Page = Route.options.component
  if (!Page) throw new Error('projects route has no component')
  render(<Page />)
}

describe('page banners', () => {
  beforeEach(() => {
    projects.list = []
  })

  it('shows the projects hero with its action when there are no projects', () => {
    renderProjects()
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'New project' })).toBeTruthy()
  })

  it('shows the compact projects banner once projects exist', () => {
    projects.list = [{ id: 'p1' }]
    renderProjects()
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
    expect(screen.queryByTestId('page-banner-full')).toBeNull()
  })

  it('keeps the Import and Connect Git actions in the apps hero', () => {
    render(<AppsEmptyActions />)
    expect(
      screen.getByRole('link', { name: 'Import' }).getAttribute('href'),
    ).toBe('/settings/import-platform')
    expect(
      screen.getByRole('link', { name: 'Connect Git' }).getAttribute('href'),
    ).toBe('/settings/github-app')
  })
})
