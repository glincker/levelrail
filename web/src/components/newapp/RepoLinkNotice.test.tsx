import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { NewAppI18n } from '../../test/newAppI18n'
import { parseRepoLink } from '../../lib/deployRepoLink'
import { RepoLinkNotice } from './RepoLinkNotice'
import { RepoLinkSummary } from './RepoLinkSummary'
import { DeployLinkDialog } from './DeployLinkDialog'

vi.mock('../../hooks/useBrand', () => ({
  useBrand: () => ({ Name: 'Levelrail' }),
}))

describe('RepoLinkNotice', () => {
  it('tells the reader the form was prefilled and must be reviewed', () => {
    render(
      <NewAppI18n>
        <RepoLinkNotice ignored={[]} />
      </NewAppI18n>,
    )
    expect(
      screen.getByText('Prefilled from a link, review before deploying'),
    ).toBeTruthy()
    expect(screen.queryByText(/were ignored/)).toBeNull()
  })

  it('names rejected params and renders them as text', () => {
    const prefill = parseRepoLink({
      repo: 'javascript:alert(1)',
      '<img src=x onerror=alert(1)>': '1',
    })
    const { container } = render(
      <NewAppI18n>
        <RepoLinkNotice ignored={prefill.ignored} />
      </NewAppI18n>,
    )
    expect(screen.getByText(/repo, <img src=x/)).toBeTruthy()
    expect(container.querySelector('img')).toBeNull()
  })
})

describe('RepoLinkSummary', () => {
  it('shows the validated values and the default branch fallback', () => {
    const prefill = parseRepoLink({
      repo: 'https://github.com/you/app',
      name: 'app',
      port: '3000',
    })
    render(
      <NewAppI18n>
        <RepoLinkSummary prefill={prefill} />
      </NewAppI18n>,
    )
    expect(screen.getByText('https://github.com/you/app')).toBeTruthy()
    expect(screen.getByText('Default branch')).toBeTruthy()
    expect(screen.getByText('3000')).toBeTruthy()
  })
})

describe('DeployLinkDialog', () => {
  it('builds the link and snippets for a known repo', () => {
    render(
      <NewAppI18n>
        <DeployLinkDialog
          open
          onOpenChange={() => undefined}
          repo="https://github.com/you/app.git"
          branch="main"
          name="app"
        />
      </NewAppI18n>,
    )
    const link = screen.getByLabelText('Link').textContent ?? ''
    expect(link).toContain(
      '/deploy?repo=https%3A%2F%2Fgithub.com%2Fyou%2Fapp.git&branch=main&name=app',
    )
    expect(screen.getByLabelText('Markdown').textContent).toContain(
      '[![Deploy to Levelrail]',
    )
    expect(screen.getByLabelText('HTML').textContent).toContain(
      '&amp;branch=main',
    )
  })

  it('asks for a repo on the create page and rejects credentials', () => {
    render(
      <NewAppI18n>
        <DeployLinkDialog open onOpenChange={() => undefined} />
      </NewAppI18n>,
    )
    expect(screen.queryByLabelText('Link')).toBeNull()
    expect(screen.getByLabelText('Repository URL')).toBeTruthy()
  })
})
