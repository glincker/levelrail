import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LoadBalancerExportDialog } from './LoadBalancerExportDialog'

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

describe('LoadBalancerExportDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('previews the selected format, lists warnings and copies the body', async () => {
    const requested: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = urlOf(input)
        requested.push(url)
        const format = new URL(url, 'http://x').searchParams.get('format')
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({
              format,
              filename: `demo.${format}`,
              content_type: 'text/plain',
              body: `body for ${format}`,
              warnings: format === 'cdk' ? ['no ALB equivalent'] : [],
            }),
        } as unknown as Response)
      }),
    )
    const user = userEvent.setup()
    const writeText = vi.spyOn(navigator.clipboard, 'writeText')
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LoadBalancerExportDialog appName="demo" />
      </QueryClientProvider>,
    )

    await user.click(screen.getByRole('button', { name: 'Export' }))
    expect(await screen.findByText('body for terraform')).toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'AWS CDK' }))
    expect(await screen.findByText('body for cdk')).toBeInTheDocument()
    expect(screen.getByText('no ALB equivalent')).toBeInTheDocument()
    expect(requested.at(-1)).toContain('format=cdk')

    await user.click(screen.getByRole('button', { name: 'Copy' }))
    expect(writeText).toHaveBeenCalledWith('body for cdk')
    expect(screen.getByRole('button', { name: 'Download' })).toBeEnabled()
  })
})
