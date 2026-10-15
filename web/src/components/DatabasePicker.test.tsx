import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { buildPickerEntries } from '@/lib/databaseCatalog'
import { DatabasePicker } from './DatabasePicker'
import { BRAND_ICON_NAMES } from './BrandIcon'
import { DatabasesI18n } from '../test/databasesI18n'

const ENGINES = [
  { id: 'postgres', label: 'Postgres' },
  { id: 'redis', label: 'Redis' },
  { id: 'meilisearch', label: 'Meilisearch' },
  { id: 'qdrant', label: 'Qdrant' },
  { id: 'typesense', label: 'Typesense' },
  { id: 'memcached', label: 'Memcached' },
]

function renderPicker(query = '') {
  render(
    <DatabasePicker
      entries={buildPickerEntries(ENGINES, BRAND_ICON_NAMES)}
      query={query}
      selectedKey={null}
      onSelect={vi.fn()}
      onClearQuery={vi.fn()}
    />,
    { wrapper: DatabasesI18n },
  )
}

describe('DatabasePicker search and cache engines', () => {
  it('groups the search engines under Search and vector', async () => {
    const user = userEvent.setup()
    renderPicker()
    await user.click(screen.getByRole('button', { name: 'Search and vector' }))
    for (const key of ['meilisearch', 'qdrant', 'typesense', 'pgvector']) {
      expect(screen.getByTestId(`engine-card-${key}`)).toBeInTheDocument()
    }
    expect(
      screen.queryByTestId('engine-card-memcached'),
    ).not.toBeInTheDocument()
  })

  it('puts Memcached under Key-value and cache', async () => {
    const user = userEvent.setup()
    renderPicker()
    await user.click(
      screen.getByRole('button', { name: 'Key-value and cache' }),
    )
    expect(screen.getByTestId('engine-card-memcached')).toBeInTheDocument()
    expect(screen.queryByTestId('engine-card-qdrant')).not.toBeInTheDocument()
  })

  it('labels backups honestly per engine', () => {
    renderPicker()
    const meili = screen.getByTestId('engine-card-meilisearch')
    expect(meili).toHaveTextContent('HTTP API')
    expect(meili).toHaveTextContent('Snapshots')
    const cache = screen.getByTestId('engine-card-memcached')
    expect(cache).toHaveTextContent('No persistence')
    expect(cache).not.toHaveTextContent('Backups')
    expect(cache).not.toHaveTextContent('Snapshots')
    expect(screen.getByTestId('engine-card-postgres')).toHaveTextContent(
      'Backups',
    )
  })

  it('finds an engine by its tagline', () => {
    renderPicker('embeddings')
    expect(screen.getByTestId('engine-card-qdrant')).toBeInTheDocument()
    expect(
      screen.queryByTestId('engine-card-memcached'),
    ).not.toBeInTheDocument()
  })
})
