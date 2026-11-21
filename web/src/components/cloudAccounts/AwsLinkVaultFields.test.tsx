import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { INITIAL_WIZARD, scopeIssues } from '../../lib/cloudAccounts'
import { AwsLinkVaultFields } from './AwsLinkVaultFields'

describe('AwsLinkVaultFields', () => {
  it('hides the vault inputs until the option is on', async () => {
    const onChange = vi.fn()
    render(
      <AwsLinkVaultFields
        state={INITIAL_WIZARD}
        onChange={onChange}
        issues={new Set()}
      />,
    )
    expect(screen.queryByLabelText('wizard.vault.address')).toBeNull()
    await userEvent.click(screen.getByRole('checkbox'))
    expect(onChange).toHaveBeenCalledWith({ useVault: true })
  })

  it('shows mount, role, namespace and token fields with issues', () => {
    const state = { ...INITIAL_WIZARD, useVault: true }
    render(
      <AwsLinkVaultFields
        state={state}
        onChange={vi.fn()}
        issues={new Set(scopeIssues(state))}
      />,
    )
    expect(screen.getByLabelText('wizard.vault.mount')).toHaveValue('aws')
    expect(screen.getByLabelText('wizard.vault.role')).toHaveValue('levelrail')
    expect(screen.getByLabelText('wizard.vault.namespace')).toBeTruthy()
    expect(screen.getByLabelText('wizard.vault.tokenRef')).toBeTruthy()
    expect(screen.getByText('wizard.issues.badVaultAddress')).toBeTruthy()
  })
})
