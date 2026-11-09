import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SavedViewsMenu } from './SavedViewsMenu'
import { sanitizeViewFilter } from '../lib/savedViews'

afterEach(() => vi.unstubAllGlobals())

const hostileName = '<img src=x onerror="alert(1)">team view'

function stubViews() {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            views: [
              {
                id: 'sv_1',
                name: hostileName,
                list: 'apps',
                filter: {
                  q: 'api',
                  status: 'failing',
                  redirect: '/settings',
                  __proto__: 'x',
                  tags: 7,
                },
                shared: 'team',
                mine: false,
                can_delete: false,
              },
            ],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    ),
  )
}

describe('SavedViewsMenu', () => {
  it('applies only allowlisted params and renders the name as text', async () => {
    stubViews()
    const onApply = vi.fn()
    const { container } = render(
      <QueryClientProvider client={new QueryClient()}>
        <SavedViewsMenu list="apps" current={{}} onApply={onApply} />
      </QueryClientProvider>,
    )
    fireEvent.click(
      screen.getByRole('button', { name: /Views|savedViews\.button/ }),
    )
    const item = await screen.findByText(hostileName)
    expect(container.ownerDocument.querySelector('img')).toBeNull()
    expect(container.ownerDocument.querySelector('[onerror]')).toBeNull()
    fireEvent.click(item)
    expect(onApply).toHaveBeenCalledWith({ q: 'api', status: 'failing' })
  })

  it('drops keys that belong to another list', () => {
    expect(
      sanitizeViewFilter('databases', {
        engine: 'postgres',
        env: 'prod',
        labels: 'tier=db',
      }),
    ).toEqual({ engine: 'postgres', labels: 'tier=db' })
    expect(sanitizeViewFilter('apps', null)).toEqual({})
  })
})
