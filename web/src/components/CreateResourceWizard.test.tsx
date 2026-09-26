import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { Button } from '@/components/ui/button'
import { CreateResourceWizard } from './CreateResourceWizard'

// Step 2's real field components each carry their own mutation/network
// setup (covered by their own test files); stubbed here to plain
// placeholders so this file only exercises CreateResourceWizard's own
// step routing (which option opens which form, and whether
// `initialSelected` skips step 1 entirely).
vi.mock('./BrowseTemplatesFields', () => ({
  BrowseTemplatesFields: () => <div>browse-templates-fields-stub</div>,
}))
vi.mock('./CreateAppFields', () => ({
  CreateAppFields: () => <div>create-app-fields-stub</div>,
}))
vi.mock('./CreateAppFromGitFields', () => ({
  CreateAppFromGitFields: () => <div>create-app-from-git-fields-stub</div>,
}))
vi.mock('./CreateComposeFields', () => ({
  CreateComposeFields: () => <div>create-compose-fields-stub</div>,
}))
vi.mock('./CreateDatabaseFields', () => ({
  CreateDatabaseFields: () => <div>create-database-fields-stub</div>,
}))

vi.mock('./ImportFrontDoor', () => ({
  ImportFrontDoor: ({
    onPlan,
  }: {
    onPlan: (p: unknown, r: unknown) => void
  }) => (
    <button
      type="button"
      onClick={() => {
        onPlan({ source: 'image' }, { text: 'nginx' })
      }}
    >
      front-door-stub
    </button>
  ),
}))
vi.mock('./ImportPlanPreview', () => ({
  ImportPlanPreview: ({ onBack }: { onBack: () => void }) => (
    <button type="button" onClick={onBack}>
      plan-preview-stub
    </button>
  ),
}))

vi.mock('../queries/databaseEngines', () => ({
  useDatabaseEnginesOptional: () => ({ data: [] }),
}))

describe('CreateResourceWizard', () => {
  it('opens to step 1s picker by default', async () => {
    const user = userEvent.setup()
    render(<CreateResourceWizard trigger={<Button>New resource</Button>} />)
    await user.click(screen.getByRole('button', { name: 'New resource' }))
    expect(
      screen.getByRole('heading', { name: 'New resource' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Pick a starting point.')).toBeInTheDocument()
  })

  it('shows the plan preview once the front door returns a plan, and goes back', async () => {
    const user = userEvent.setup()
    render(
      <CreateResourceWizard
        scope="applications"
        trigger={<Button>New app</Button>}
      />,
    )
    await user.click(screen.getByRole('button', { name: 'New app' }))
    await user.click(screen.getByRole('button', { name: 'front-door-stub' }))
    expect(screen.getByText('Deployment plan')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'plan-preview-stub' }))
    expect(screen.getByText('Pick a starting point.')).toBeInTheDocument()
  })

  it('jumps straight to the template step when initialSelected is set', async () => {
    const user = userEvent.setup()
    render(
      <CreateResourceWizard
        scope="applications"
        initialSelected="browse-templates"
        trigger={<Button>Start from a template</Button>}
      />,
    )
    await user.click(
      screen.getByRole('button', { name: 'Start from a template' }),
    )
    expect(
      screen.getByRole('heading', { name: /New app from a template/ }),
    ).toBeInTheDocument()
    expect(screen.getByText('browse-templates-fields-stub')).toBeInTheDocument()
    expect(screen.queryByText('Pick a starting point.')).not.toBeInTheDocument()
  })

  it('re-opens back to the template step after closing, not step 1', async () => {
    const user = userEvent.setup()
    render(
      <CreateResourceWizard
        scope="applications"
        initialSelected="browse-templates"
        trigger={<Button>Start from a template</Button>}
      />,
    )
    const trigger = screen.getByRole('button', {
      name: 'Start from a template',
    })
    await user.click(trigger)
    await user.keyboard('{Escape}')
    await user.click(trigger)
    expect(
      screen.getByRole('heading', { name: /New app from a template/ }),
    ).toBeInTheDocument()
  })
})
