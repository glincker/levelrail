import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../../test/NewSurfacesProviders'
import { FunctionsPage } from './FunctionsPage'

const state = vi.hoisted(() => ({
  items: [] as { name: string }[],
}))

vi.mock('../../queries/functions', () => ({
  useFunctions: () => ({
    isPending: false,
    isError: false,
    isSuccess: true,
    data: { functions: state.items },
  }),
}))
vi.mock('../PageBanner', () => ({ PageBanner: () => <div>banner</div> }))
vi.mock('../banners/FunctionsDiagram', () => ({
  FunctionsDiagram: () => null,
}))
vi.mock('./DeployFunctionDialog', () => ({
  DeployFunctionDialog: () => null,
}))
vi.mock('./FunctionsTable', () => ({
  FunctionsTable: () => <div>table</div>,
}))
vi.mock('./FunctionWorkbench', () => ({
  FunctionWorkbench: () => <div>workbench</div>,
}))
vi.mock('./FunctionTriggers', () => ({
  FunctionTriggers: () => <div>triggers</div>,
}))
vi.mock('./FunctionTemplates', () => ({
  FunctionTemplates: () => <div>templates</div>,
}))

function renderPage() {
  return render(
    <NewSurfacesProviders>
      <FunctionsPage />
    </NewSurfacesProviders>,
  )
}

describe('FunctionsPage states', () => {
  beforeEach(() => {
    state.items = []
  })
  it('shows only the hero and templates with no functions', () => {
    renderPage()
    expect(screen.getByText('banner')).toBeTruthy()
    expect(screen.getByText('templates')).toBeTruthy()
    expect(screen.queryByText('workbench')).toBeNull()
    expect(screen.queryByText('triggers')).toBeNull()
    expect(screen.queryByText('table')).toBeNull()
  })
  it('shows the full layout once functions exist', () => {
    state.items = [{ name: 'hello' }]
    renderPage()
    expect(screen.getByText('table')).toBeTruthy()
    expect(screen.getByText('workbench')).toBeTruthy()
    expect(screen.getByText('triggers')).toBeTruthy()
    expect(screen.getByText('templates')).toBeTruthy()
  })
})
