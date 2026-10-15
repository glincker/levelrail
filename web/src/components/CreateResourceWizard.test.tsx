import { render as rtlRender, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { Button } from '@/components/ui/button'
import { CreateResourceWizard } from './CreateResourceWizard'
import { DatabasesI18n } from '../test/databasesI18n'

const render = (ui: ReactElement) => rtlRender(ui, { wrapper: DatabasesI18n })

// Step 2's real field components each carry their own mutation/network
// setup (covered by their own test files); stubbed here to plain
// placeholders so this file only exercises CreateResourceWizard's own
// step routing (which option opens which form, and whether
// `initialSelected` skips step 1 entirely).
vi.mock('./BrowseTemplatesFields', () => ({
  BrowseTemplatesFields: ({
    onViewChange,
  }: {
    onViewChange?: (view: 'grid' | 'detail') => void
  }) => (
    <div>
      browse-templates-fields-stub
      <button
        type="button"
        onClick={() => {
          onViewChange?.('detail')
        }}
      >
        stub-pick-template
      </button>
    </div>
  ),
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
  CreateDatabaseFields: ({
    engine,
    flavor,
  }: {
    engine: string
    flavor?: string | null
  }) => (
    <div>
      create-database-fields-stub {engine} {flavor ?? 'plain'}
    </div>
  ),
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

const engineFixture = vi.hoisted(() => ({
  data: [] as { id: string; label: string; default_version: string }[],
}))

vi.mock('../queries/databaseEngines', () => ({
  useDatabaseEnginesOptional: () => engineFixture,
}))

const ALL_ENGINES = [
  { id: 'postgres', label: 'Postgres', default_version: '16' },
  { id: 'mysql', label: 'MySQL', default_version: '8' },
  { id: 'redis', label: 'Redis', default_version: '7' },
  { id: 'valkey', label: 'Valkey', default_version: '8' },
  { id: 'clickhouse', label: 'ClickHouse', default_version: '24.8' },
]

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

  it('widens for the template catalog grid, narrows once a template is picked', async () => {
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
    // Dialog content renders into a portal appended to document.body, not
    // under the local render container, hence document.querySelector here.
    expect(document.querySelector('.max-w-6xl')).toBeInTheDocument()
    expect(document.querySelector('.max-w-2xl')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'stub-pick-template' }))
    expect(document.querySelector('.max-w-2xl')).toBeInTheDocument()
    expect(document.querySelector('.max-w-6xl')).not.toBeInTheDocument()
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

  describe('database picker', () => {
    async function openDatabases(user: ReturnType<typeof userEvent.setup>) {
      render(
        <CreateResourceWizard
          scope="databases"
          trigger={<Button>New database</Button>}
        />,
      )
      await user.click(screen.getByRole('button', { name: 'New database' }))
    }

    it('lists engines and Postgres flavors with distinct taglines', async () => {
      engineFixture.data = ALL_ENGINES
      const user = userEvent.setup()
      await openDatabases(user)
      expect(screen.getByTestId('engine-card-valkey')).toHaveTextContent(
        'Redis compatible',
      )
      expect(screen.getByTestId('engine-card-timescaledb')).toHaveTextContent(
        'time series',
      )
      expect(screen.getByTestId('engine-card-postgis')).toHaveTextContent(
        'Postgres compatible',
      )
      expect(
        screen.queryByRole('button', { name: 'Expand to fullscreen' }),
      ).not.toBeInTheDocument()
    })

    it('filters by family', async () => {
      engineFixture.data = ALL_ENGINES
      const user = userEvent.setup()
      await openDatabases(user)
      await user.click(
        screen.getByRole('button', { name: 'Key-value and cache' }),
      )
      expect(screen.getByTestId('engine-card-redis')).toBeInTheDocument()
      expect(
        screen.queryByTestId('engine-card-postgres'),
      ).not.toBeInTheDocument()
    })

    it('shows real empty-search copy and clears it', async () => {
      engineFixture.data = ALL_ENGINES
      const user = userEvent.setup()
      await openDatabases(user)
      await user.type(screen.getByLabelText('Search resources'), 'zzzz')
      expect(screen.getByText(/No database matches "zzzz"/)).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: 'Clear search' }))
      expect(screen.getByTestId('engine-card-postgres')).toBeInTheDocument()
    })

    it('moves focus with arrow keys and chooses with Enter', async () => {
      engineFixture.data = ALL_ENGINES
      const user = userEvent.setup()
      await openDatabases(user)
      screen.getByTestId('engine-card-postgres').focus()
      await user.keyboard('{ArrowRight}')
      expect(screen.getByTestId('engine-card-postgis')).toHaveFocus()
      await user.keyboard('{Enter}')
      expect(
        screen.getByText('create-database-fields-stub postgres postgis'),
      ).toBeInTheDocument()
    })

    it('routes a database that is not in the list to the app from image flow', async () => {
      engineFixture.data = ALL_ENGINES
      const user = userEvent.setup()
      await openDatabases(user)
      await user.click(
        screen.getByRole('button', {
          name: /Not in the list\? Run any database image/,
        }),
      )
      expect(
        screen.getByRole('heading', { name: 'New app from an image' }),
      ).toBeInTheDocument()
    })

    it('shows the picked engine in the step header and goes back to the picker', async () => {
      engineFixture.data = ALL_ENGINES
      const user = userEvent.setup()
      await openDatabases(user)
      await user.click(screen.getByTestId('engine-card-valkey'))
      expect(
        screen.getByRole('heading', { name: /New Valkey database/ }),
      ).toBeInTheDocument()
      expect(screen.getByText('Redis compatible')).toBeInTheDocument()
      await user.click(
        screen.getByRole('button', { name: 'All starting points' }),
      )
      expect(screen.getByTestId('engine-card-valkey')).toBeInTheDocument()
    })

    it('opens a flavor card on the postgres engine with its flavor', async () => {
      engineFixture.data = ALL_ENGINES
      const user = userEvent.setup()
      await openDatabases(user)
      await user.click(screen.getByTestId('engine-card-timescaledb'))
      expect(
        screen.getByRole('heading', { name: /New TimescaleDB database/ }),
      ).toBeInTheDocument()
      expect(
        screen.getByText('create-database-fields-stub postgres timescaledb'),
      ).toBeInTheDocument()
    })
  })
})
