import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PipelineReportLink } from './PipelineReportLink'

describe('PipelineReportLink', () => {
  it('shows a dash when nothing was reported', () => {
    render(<PipelineReportLink />)
    expect(screen.getByText('-')).toBeTruthy()
  })

  it('links to the commit on the forge', () => {
    render(
      <PipelineReportLink
        report={{
          provider: 'github',
          state: 'success',
          url: 'https://github.com/o/r/commit/abc',
        }}
      />,
    )
    const link = screen.getByRole('link', { name: /Reported to GitHub/ })
    expect(link.getAttribute('href')).toBe('https://github.com/o/r/commit/abc')
  })

  it('shows the warning when the post failed', () => {
    render(
      <PipelineReportLink
        report={{
          provider: 'gitlab',
          warning: 'rate limited by the git provider',
        }}
      />,
    )
    expect(screen.getByText(/Not reported to GitLab/)).toBeTruthy()
    expect(screen.queryByRole('link')).toBeNull()
  })
})
