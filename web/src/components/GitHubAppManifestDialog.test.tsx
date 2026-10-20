import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import { GitHubAppOwnerFields } from './GitHubAppManifestDialog'
import {
  isValidOrgLogin,
  registerStartParams,
  type GitHubAppOwnerKind,
} from '../lib/githubAppOwner'
import settingsEn from '../locales/en/settings.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function Harness() {
  const [kind, setKind] = useState<GitHubAppOwnerKind>('personal')
  const [login, setLogin] = useState('')
  const [allow, setAllow] = useState(false)
  return (
    <I18nextProvider i18n={testI18n}>
      <GitHubAppOwnerFields
        ownerKind={kind}
        onOwnerKindChange={setKind}
        orgLogin={login}
        onOrgLoginChange={setLogin}
        allowOthers={allow}
        onAllowOthersChange={setAllow}
      />
      <output data-testid="params">
        {registerStartParams({
          name: '',
          instanceURL: '',
          ownerKind: kind,
          orgLogin: login,
          allowOthers: allow,
        }).toString()}
      </output>
    </I18nextProvider>
  )
}

describe('GitHubAppOwnerFields', () => {
  it('hides the org login until an organization is chosen', () => {
    render(<Harness />)
    expect(screen.queryByLabelText('Organization login')).toBeNull()
    fireEvent.click(screen.getByLabelText('An organization'))
    expect(screen.getByLabelText('Organization login')).toBeInTheDocument()
  })

  it('emits owner and public only when chosen', () => {
    render(<Harness />)
    expect(screen.getByTestId('params').textContent).toBe('')
    fireEvent.click(screen.getByLabelText('An organization'))
    fireEvent.change(screen.getByLabelText('Organization login'), {
      target: { value: 'acme-inc' },
    })
    fireEvent.click(
      screen.getByRole('checkbox', {
        name: 'Allow other accounts and organizations to install it',
      }),
    )
    expect(screen.getByTestId('params').textContent).toBe(
      'owner=acme-inc&public=true',
    )
  })

  it('drops a stale org login when switching back to personal', () => {
    render(<Harness />)
    fireEvent.click(screen.getByLabelText('An organization'))
    fireEvent.change(screen.getByLabelText('Organization login'), {
      target: { value: 'acme-inc' },
    })
    fireEvent.click(screen.getByLabelText('My personal account'))
    expect(screen.getByTestId('params').textContent).toBe('')
  })

  it('flags an invalid login', () => {
    render(<Harness />)
    fireEvent.click(screen.getByLabelText('An organization'))
    fireEvent.change(screen.getByLabelText('Organization login'), {
      target: { value: 'bad/login' },
    })
    expect(screen.getByText(/letters, numbers and hyphens/)).toBeInTheDocument()
  })
})

describe('isValidOrgLogin', () => {
  it.each([
    ['acme', true],
    ['acme-inc', true],
    ['-acme', false],
    ['a/b', false],
    ['', false],
    ['a'.repeat(40), false],
  ])('%s -> %s', (login, want) => {
    expect(isValidOrgLogin(login)).toBe(want)
  })
})
