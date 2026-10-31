import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import ssoEn from '../../../locales/en/sso.json'
import type {
  OAuthSSOPolicy,
  SSOEvaluation,
  SSOTestResult,
} from '../../../queries/ssoOidc'
import { emailDomain, splitDomains } from '../../../queries/ssoOidc'
import { OIDCTestResult } from './OIDCTestResult'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['sso'],
  defaultNS: 'sso',
  resources: { en: { sso: ssoEn } },
  interpolation: { escapeValue: false },
})

const policy: OAuthSSOPolicy = {
  provider: 'oidc',
  name: 'Okta',
  enabled: true,
  groups_supported: true,
  groups_claim: 'groups',
  sync_roles: false,
  default_role_id: 'role_viewer',
  jit_provisioning: false,
  allowed_domains: ['corp.test'],
}

const evaluation: SSOEvaluation = {
  provider: 'oidc',
  email: 'a@corp.test',
  email_verified: true,
  groups_claim: 'groups',
  groups_found: false,
  groups: [],
  group_like_claims: ['roles'],
  role_id: 'role_viewer',
  role_source: 'default',
  account_state: 'not_provisioned',
  sync_roles: false,
  allowed: false,
  refusal_code: 'sso_not_provisioned',
  issues: [
    {
      code: 'wrong_claim_name',
      fix: { kind: 'auto', action: 'set_groups_claim', value: 'roles' },
    },
    { code: 'not_provisioned', fix: { kind: 'auto', action: 'enable_jit' } },
    { code: 'email_unverified', fix: { kind: 'manual' } },
  ],
}

function renderResult(result: SSOTestResult) {
  const qc = createTestQueryClient()
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <OIDCTestResult result={result} policy={policy} roles={[]} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('OIDCTestResult', () => {
  it('offers a typed fix button that saves the corrected policy', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(policy),
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderResult({ status: 'ok', evaluation })

    expect(screen.getByText('This sign-in would be refused')).toBeTruthy()
    expect(screen.getByText('Fix at the identity provider')).toBeTruthy()
    fireEvent.click(screen.getByText('Use roles as the groups claim'))

    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ]
    expect(url).toBe('/api/v1/sso/oidc/oidc')
    expect(init.method).toBe('PUT')
    expect(JSON.parse(init.body as string)).toMatchObject({
      groups_claim: 'roles',
    })
  })

  it('says a failed test login failed without leaking detail', () => {
    renderResult({ status: 'failed', failure_code: 'exchange_failed' })
    expect(screen.getByText('The test login did not finish')).toBeTruthy()
  })
})

describe('sign-in domain helpers', () => {
  it('sends only a valid lowercase domain', () => {
    expect(emailDomain('Person@Corp.Test')).toBe('corp.test')
    expect(emailDomain('no-at-sign')).toBe('')
    expect(emailDomain('a@nodots')).toBe('')
    expect(emailDomain('a@bad domain.test')).toBe('')
  })

  it('splits allowed domains on commas and spaces', () => {
    expect(splitDomains('A.test, b.test  c.test,')).toEqual([
      'a.test',
      'b.test',
      'c.test',
    ])
  })
})
