import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GitSiteMode from './GitSiteMode'
import type { RepoPreview } from '../../../queries/repoPreview'

const h = vi.hoisted(() => ({
  preview: undefined as RepoPreview | undefined,
  create: vi.fn(),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
  useNavigate: () => vi.fn(),
}))
vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({
    data: h.preview,
    isPending: false,
    isError: false,
  }),
}))
vi.mock('../../../hooks/useConnectedRepos', () => ({
  useConnectedRepos: () => ({
    repos: [],
    isLoading: false,
    hasError: false,
  }),
}))
vi.mock('../../../queries/gitRepos', () => ({
  useRefreshGitRepos: () => ({ isPending: false, mutate: vi.fn() }),
}))
vi.mock('../../../queries/gitSourceFirstDeploy', () => ({
  useFirstDeployResult: () => null,
}))
vi.mock('../../../queries/pagesGit', async (orig) => ({
  ...(await orig<typeof import('../../../queries/pagesGit')>()),
  createGitSite: h.create,
}))
vi.mock('../../newapp/RepoList', () => ({ RepoList: () => null }))
vi.mock('../../BrandIcon', () => ({
  BrandIcon: ({ name }: { name: string }) => (
    <span data-testid="logo">{name}</span>
  ),
  isBrandIconName: () => true,
}))
vi.mock('../SiteSuccess', () => ({ SiteSuccess: () => null }))

function preview(buildType: string): RepoPreview {
  return {
    kind: 'public',
    repo_url: 'https://github.com/a/site',
    name: 'site',
    default_branch: 'main',
    branches: ['main', 'dev'],
    analysis: {
      branch: 'main',
      stack: { runtime: 'node', framework: 'Vite', evidence: '' },
      build: { type: buildType, evidence: '', alternatives: [] },
      detection: {
        framework: 'Vite',
        build_command: 'npm run build',
        output_dir: 'dist',
      },
    },
  } as unknown as RepoPreview
}

function mount() {
  render(
    <GitSiteMode
      name="site"
      nameInvalid={false}
      onRepoName={vi.fn()}
      onBusyChange={vi.fn()}
      onClose={vi.fn()}
    />,
  )
  fireEvent.change(screen.getByLabelText('git.repo'), {
    target: { value: 'https://github.com/a/site' },
  })
}

describe('GitSiteMode', () => {
  beforeEach(() => {
    h.create.mockReset().mockResolvedValue('d1')
  })

  it('prefills the detected build settings and creates with them', async () => {
    h.preview = preview('static')
    mount()
    expect((await screen.findByTestId('logo')).textContent).toBe('vite')
    expect(screen.getByLabelText('git.command')).toHaveProperty(
      'value',
      'npm run build',
    )
    expect(screen.getByLabelText('git.output')).toHaveProperty('value', 'dist')
    fireEvent.change(screen.getByLabelText('git.output'), {
      target: { value: 'public' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'git.create' }))
    await waitFor(() => expect(h.create).toHaveBeenCalled())
    expect(h.create.mock.calls[0]?.[0]).toMatchObject({
      name: 'site',
      buildCommand: 'npm run build',
      outputDir: 'public',
      resume: false,
    })
  })

  it('blocks create for a server app until forced', async () => {
    h.preview = preview('railpack')
    mount()
    expect(await screen.findByText('git.serverWarning')).toBeTruthy()
    const create = screen.getByRole('button', { name: 'git.create' })
    expect(create).toHaveProperty('disabled', true)
    fireEvent.click(screen.getByRole('button', { name: 'git.forceStatic' }))
    expect(create).toHaveProperty('disabled', false)
  })
})
