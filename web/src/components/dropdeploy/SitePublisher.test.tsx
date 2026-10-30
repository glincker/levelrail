import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../../lib/apiError'
import { NewAppI18n } from '../../test/newAppI18n'
import { SitePublisher } from './SitePublisher'
import { createTestQueryClient } from '@/test/testQueryClient'

const publish = vi.fn<(input: unknown) => Promise<unknown>>()
let failWith: Error | null = null
// Rejections bypass vi.fn: its result tracking re-raises a rejected promise as unhandled.
vi.mock('../../queries/dropDeploy', () => ({
  publishDroppedSite: (input: unknown) =>
    failWith ? Promise.reject(failWith) : publish(input),
}))

function folderFile(path: string): File {
  const f = new File(['x'], path.split('/').pop() ?? path)
  Object.defineProperty(f, 'webkitRelativePath', { value: path })
  return f
}

function renderPublisher(appName?: string) {
  const qc = createTestQueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <NewAppI18n>
        <SitePublisher appName={appName} />
      </NewAppI18n>
    </QueryClientProvider>,
  )
}

function pickFolder(files: File[]) {
  fireEvent.change(screen.getByTestId('site-folder-input'), {
    target: { files },
  })
}

describe('SitePublisher', () => {
  beforeEach(() => {
    publish.mockReset()
    failWith = null
  })

  it('previews a dropped folder and publishes to the existing app', async () => {
    publish.mockResolvedValue({
      deployId: 'dep_1',
      upload: { hash: 'h', file_count: 2 },
    })
    renderPublisher('docs')
    pickFolder([folderFile('site/index.html'), folderFile('site/a.css')])
    expect(await screen.findByText('site')).toBeTruthy()
    expect(screen.getByText('2')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Publish' }))
    await waitFor(() => expect(publish).toHaveBeenCalled())
    expect(publish.mock.calls[0]?.[0]).toMatchObject({
      appName: 'docs',
      create: false,
    })
    expect(await screen.findByText(/Published\./)).toBeTruthy()
  })

  it('names a new site from the folder and lets the user override it', async () => {
    publish.mockResolvedValue({
      deployId: 'dep_1',
      upload: { hash: 'h', file_count: 1 },
    })
    renderPublisher()
    pickFolder([folderFile('My Blog/index.html')])
    const input = await screen.findByLabelText<HTMLInputElement>('Site name')
    expect(input.value).toBe('my-blog')
    fireEvent.change(input, { target: { value: 'journal' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create and publish' }))
    await waitFor(() => expect(publish).toHaveBeenCalled())
    expect(publish.mock.calls[0]?.[0]).toMatchObject({
      appName: 'journal',
      create: true,
    })
  })

  it('warns when there is no index.html', async () => {
    renderPublisher('docs')
    pickFolder([folderFile('dist/assets/a.js')])
    expect(await screen.findByText(/No index\.html at the top/)).toBeTruthy()
  })

  it('explains an empty drop', async () => {
    renderPublisher('docs')
    pickFolder([folderFile('s/node_modules/a.js')])
    expect(
      await screen.findByText(/Nothing to publish in that drop/),
    ).toBeTruthy()
  })

  it('shows an actionable error with the server detail and offers a retry', async () => {
    failWith = new ApiError(413, 'site is too large once unpacked')
    renderPublisher('docs')
    pickFolder([folderFile('site/index.html')])
    fireEvent.click(await screen.findByRole('button', { name: 'Publish' }))
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('over the upload limits')
    expect(alert.textContent).toContain('too large once unpacked')
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy()
  })

  it('accepts a zip without a file count', async () => {
    renderPublisher('docs')
    fireEvent.change(screen.getByTestId('site-zip-input'), {
      target: { files: [new File(['PK'], 'site.zip')] },
    })
    expect(await screen.findByText('site.zip')).toBeTruthy()
    expect(screen.queryByText('Files')).toBeNull()
  })
})
