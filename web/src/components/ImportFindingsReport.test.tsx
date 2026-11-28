import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { AllI18n } from '../test/allI18n'
import { ImportFindingsReport } from './ImportFindingsReport'

describe('ImportFindingsReport', () => {
  it('renders nothing without findings', () => {
    const { container } = render(
      <AllI18n>
        <ImportFindingsReport findings={[]} />
      </AllI18n>,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('groups rows by severity with blockers first', () => {
    render(
      <AllI18n>
        <ImportFindingsReport
          findings={[
            {
              kind: 'dropped',
              path: 'web.cap_add',
              reason: 'not supported',
              severity: 'ignored-safe',
            },
            {
              kind: 'unsupported',
              path: 'web.privileged',
              reason: 'host access',
              severity: 'blocker',
              fix: 'remove it',
            },
          ]}
        />
      </AllI18n>,
    )
    const sections = screen.getAllByRole('region')
    expect(sections[0]).toHaveAccessibleName('Blockers')
    expect(within(sections[0]!).getByText('web.privileged')).toBeInTheDocument()
    expect(screen.getByText('Fix: remove it')).toBeInTheDocument()
    expect(sections[1]).toHaveAccessibleName('Ignored, safe')
  })
})
