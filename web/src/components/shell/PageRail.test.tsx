import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import {
  PageRail,
  PageRailList,
  PageRailRow,
  PageRailSection,
} from './PageRail'

describe('PageRail', () => {
  it('renders a named section with label and value rows in order', () => {
    render(
      <PageRail>
        <PageRailSection title="Connection">
          <PageRailRow label="Status">Connected</PageRailRow>
          <PageRailRow label="Instance">https://example.test</PageRailRow>
        </PageRailSection>
      </PageRail>,
    )
    const section = screen.getByRole('region', { name: 'Connection' })
    const text = section.textContent ?? ''
    expect(text.indexOf('Status')).toBeLessThan(text.indexOf('Connected'))
    expect(text.indexOf('Connected')).toBeLessThan(text.indexOf('Instance'))
    expect(within(section).getByText('https://example.test')).toBeVisible()
  })

  it('renders list items as a list', () => {
    render(
      <PageRail>
        <PageRailSection title="Access">
          <PageRailList items={['Read repos', 'Receive webhooks']} />
        </PageRailSection>
      </PageRail>,
    )
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
  })
})
