import { describe, expect, it, vi, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ResourceNotesCard } from './ResourceNotesCard'

afterEach(() => vi.unstubAllGlobals())

describe('ResourceNotesCard', () => {
  it('renders hostile notes as inert text', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              description: '<b>Checkout</b>',
              notes:
                '<script>window.pwned=1</script>\n\n<img src=x onerror="alert(1)">\n\n[bad](javascript:alert(1)) [ok](https://runbooks.example)',
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        ),
      ),
    )
    const qc = new QueryClient()
    const { container } = render(
      <QueryClientProvider client={qc}>
        <ResourceNotesCard path="apps" name="web" />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('<b>Checkout</b>')).toBeInTheDocument()
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('[onerror]')).toBeNull()
    const links = Array.from(container.querySelectorAll('a'))
    expect(links.map((a) => a.getAttribute('href'))).toEqual([
      'https://runbooks.example',
    ])
    expect(links[0]?.getAttribute('rel')).toBe('noopener noreferrer')
  })
})
