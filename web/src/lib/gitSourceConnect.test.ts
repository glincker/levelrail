import { beforeEach, describe, expect, it, vi } from 'vitest'
import { connectGitSource } from './gitSourceConnect'
import { emptyForm } from './gitSourceForm'

const github = vi.fn<(...args: unknown[]) => Promise<Record<string, unknown>>>()
const provider =
  vi.fn<(...args: unknown[]) => Promise<Record<string, unknown>>>()

vi.mock('../queries/githubApp', () => ({
  connectGitHubRepoAsSource: (...args: unknown[]) => github(...args),
}))
vi.mock('../queries/gitConnections', () => ({
  connectRepoAsSource: (...args: unknown[]) => provider(...args),
}))
vi.mock('../queries/gitSources', () => ({ setGitSource: vi.fn() }))

beforeEach(() => {
  github.mockResolvedValue({ webhook_registered: true })
  provider.mockResolvedValue({})
})

describe('connectGitSource', () => {
  it('sends the picked subdirectory on both provider paths', async () => {
    const form = {
      ...emptyForm(),
      repoUrl: 'https://github.com/org/mono',
      baseDirectory: 'services/a',
    }
    await connectGitSource('web', {
      ...form,
      providerRef: { kind: 'github', owner: 'org', repo: 'mono' },
    })
    expect(github).toHaveBeenCalledWith(
      'org',
      'mono',
      expect.objectContaining({ base_directory: 'services/a' }),
    )
    await connectGitSource('web', {
      ...form,
      providerRef: { kind: 'gitlab', connectionId: 'gc_1', repo: '7' },
    })
    expect(provider).toHaveBeenCalledWith(
      'gc_1',
      expect.objectContaining({ base_directory: 'services/a' }),
    )
  })
})
