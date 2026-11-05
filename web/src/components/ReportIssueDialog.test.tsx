import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ReportIssueDialog } from './ReportIssueDialog'

vi.mock('@tanstack/react-router', () => ({
  useRouterState: () => ({
    pathname: '/apps/demo',
    routeId: '/_authed/apps/$name/',
  }),
}))
vi.mock('@tanstack/react-query', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-query')>()),
  useQuery: () => ({ data: { current_version: 'v0.9.1' } }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, string>) =>
      o ? `${k} ${Object.values(o).join(' ')}` : k,
  }),
}))

function href(): string {
  return (
    screen
      .getByRole('link', { name: /reportIssue.open/ })
      .getAttribute('href') ?? ''
  )
}

describe('ReportIssueDialog', () => {
  it('builds the link from edits and unticked fields', async () => {
    const user = userEvent.setup()
    render(
      <ReportIssueDialog
        open
        onOpenChange={() => undefined}
        source="error"
        supportUrl="https://github.com/acme/repo/issues"
        errorMessage="boom"
        requestId="req-9"
      />,
    )
    await waitFor(() => {
      expect(href()).toContain('template=bug_report.yml')
    })
    expect(decodeURIComponent(href())).toContain('Request ID: req-9')
    expect(decodeURIComponent(href())).toContain('version=v0.9.1')

    const title = screen.getByLabelText('reportIssue.issueTitle')
    await user.clear(title)
    await user.type(title, 'bug: custom')
    await user.click(
      screen.getByRole('checkbox', { name: /reportIssue.include.*version/ }),
    )

    const u = decodeURIComponent(href())
    expect(u).toContain('title=bug: custom')
    expect(u).not.toContain('version=')
    expect(u).toContain('component=Control plane')
  })
})
