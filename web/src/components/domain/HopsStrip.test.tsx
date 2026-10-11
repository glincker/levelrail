import { screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { HopsStrip } from './HopsStrip'
import { PreviewPanel } from './PolicyShared'
import { renderWithProviders } from './testUtils'

describe('HopsStrip', () => {
  it('renders each hop with its status and location', () => {
    renderWithProviders(
      <HopsStrip
        samples={[
          {
            url: 'http://www.example.com/a?x=1',
            hops: [
              {
                url: 'http://www.example.com/a?x=1',
                status: 308,
                location: 'https://www.example.com/a?x=1',
                note: 'automatic HTTPS',
              },
              {
                url: 'https://www.example.com/a?x=1',
                status: 301,
                location: 'https://example.com/a?x=1',
                note: 'domain redirect',
              },
              {
                url: 'https://example.com/a?x=1',
                status: 200,
                note: 'served by the app',
              },
            ],
          },
          {
            url: 'https://loop.example.com/',
            hops: [],
            error: 'redirect loop: x',
          },
        ]}
      />,
    )
    const items = screen.getAllByRole('listitem')
    const first = items[0]!
    expect(within(first).getByText('308')).toBeInTheDocument()
    expect(
      within(first).getByText('https://example.com/a?x=1'),
    ).toBeInTheDocument()
    expect(within(first).getByText('served by the app')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('redirect loop')
  })
})

describe('PreviewPanel', () => {
  it('lists steps in order and shows validation errors', () => {
    renderWithProviders(
      <PreviewPanel
        loading={false}
        steps={[
          { stage: 'geo', outcome: 'visitors from RU are stopped' },
          {
            stage: 'forwarders',
            outcome: 'proxied to app api as /users',
            final: true,
          },
        ]}
        errors={[{ field: 'headers.rules[0].name', message: 'bad name' }]}
      />,
    )
    const steps = screen.getAllByRole('listitem')
    expect(steps[0]).toHaveTextContent('visitors from RU are stopped')
    expect(steps[1]).toHaveTextContent('proxied to app api')
    expect(
      screen.getByText(/headers.rules\[0\].name: bad name/),
    ).toBeInTheDocument()
  })
})
