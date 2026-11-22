import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { NodeProviderAccounts } from './NodeProviderAccounts'
import type { NodeProviderResource } from '../../types/nodeProvision'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/x">{children}</a>,
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

const acct = {
  id: 'a1',
  label: 'oci-prod',
  status: 'active',
  auth_method: 'api_key',
  region: 'us-ashburn-1',
  needs_relogin: false,
  managed: false,
}

const base: NodeProviderResource = {
  provider: 'oracle',
  has_token: true,
  connected: true,
  accounts: [acct],
}

describe('NodeProviderAccounts', () => {
  it('lists accounts with status, links and the legacy hint', () => {
    render(<NodeProviderAccounts provider={base} />)
    expect(screen.getByText('oci-prod')).toBeTruthy()
    expect(screen.getByText('linkedAccounts.active')).toBeTruthy()
    expect(screen.getByText('linkedAccounts.manage')).toBeTruthy()
    expect(screen.getByText('linkedAccounts.add')).toBeTruthy()
    expect(screen.getByText('linkedAccounts.legacyHint')).toBeTruthy()
  })

  it('flags a relogin and hides the hint without a legacy credential', () => {
    const p = {
      ...base,
      has_token: false,
      accounts: [{ ...acct, needs_relogin: true }],
    }
    render(<NodeProviderAccounts provider={p} />)
    expect(screen.getByText('linkedAccounts.relogin')).toBeTruthy()
    expect(screen.queryByText('linkedAccounts.legacyHint')).toBeNull()
  })
})
