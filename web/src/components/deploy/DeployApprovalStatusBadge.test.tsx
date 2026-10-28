import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../../locales/en/common.json'
import { DeployApprovalStatusBadge } from './DeployApprovalStatusBadge'
import { canDecideApproval } from '../../lib/deployApprovalStatus'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { common: commonEn } },
  defaultNS: 'common',
  interpolation: { escapeValue: false },
})

function renderBadge(status: string) {
  return render(
    <I18nextProvider i18n={testI18n}>
      <DeployApprovalStatusBadge status={status} />
    </I18nextProvider>,
  )
}

describe('DeployApprovalStatusBadge', () => {
  it('shows the approving state as an in-progress neutral badge', () => {
    const { container } = renderBadge('approving')
    const badge = screen.getByText('Applying').closest('[data-status]')
    expect(badge?.getAttribute('data-status')).toBe('approving')
    expect(badge?.className).not.toMatch(
      /tone-danger|tone-success|tone-warning/,
    )
    expect(container.querySelector('.animate-spin')).not.toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('falls back to a neutral badge showing the raw status for an unknown value', () => {
    const { container } = renderBadge('quarantined')
    const badge = screen.getByText('quarantined').closest('[data-status]')
    expect(badge?.getAttribute('data-status')).toBe('quarantined')
    expect(badge?.className).toMatch(/bg-muted/)
    expect(container.querySelector('.animate-spin')).toBeNull()
  })

  it('renders the decided statuses with their labels', () => {
    renderBadge('approved')
    renderBadge('rejected')
    renderBadge('pending')
    expect(screen.getByText('Approved')).toBeTruthy()
    expect(screen.getByText('Rejected')).toBeTruthy()
    expect(screen.getByText('Pending')).toBeTruthy()
  })
})

describe('canDecideApproval', () => {
  it('allows a decision only while the approval is pending', () => {
    expect(canDecideApproval('pending')).toBe(true)
    expect(canDecideApproval('approving')).toBe(false)
    expect(canDecideApproval('approved')).toBe(false)
    expect(canDecideApproval('rejected')).toBe(false)
    expect(canDecideApproval('expired')).toBe(false)
    expect(canDecideApproval('quarantined')).toBe(false)
  })
})
