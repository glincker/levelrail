import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { CredentialDialog } from './CredentialDialog'
import accessEn from '../../locales/en/databaseAccess.json'
import type { DatabaseCredential } from '../../types/databaseAccess'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['databaseAccess'],
  defaultNS: 'databaseAccess',
  resources: { en: { databaseAccess: accessEn } },
  interpolation: { escapeValue: false },
})

const credential: DatabaseCredential = {
  username: 'tmp_ab12cd34ef',
  password: 'S3cretPassw0rdValue',
  database: 'main',
  host: 'db-main',
  port: 5432,
  sslmode: 'require',
  internal_url:
    'postgres://tmp_ab12cd34ef:S3cretPassw0rdValue@db-main:5432/main?sslmode=require',
  expires_at: '2026-10-09T12:00:00Z',
}

describe('CredentialDialog', () => {
  it('shows the password and connection string once, then discards them on close', async () => {
    const onClose = vi.fn()
    const { rerender } = render(
      <I18nextProvider i18n={testI18n}>
        <CredentialDialog credential={credential} onClose={onClose} />
      </I18nextProvider>,
    )
    expect(screen.getByText('S3cretPassw0rdValue')).toBeTruthy()
    expect(screen.getByText(credential.internal_url)).toBeTruthy()

    await userEvent.click(
      screen.getByRole('button', { name: accessEn.credential.done }),
    )
    expect(onClose).toHaveBeenCalledTimes(1)

    rerender(
      <I18nextProvider i18n={testI18n}>
        <CredentialDialog credential={null} onClose={onClose} />
      </I18nextProvider>,
    )
    expect(screen.queryByText('S3cretPassw0rdValue')).toBeNull()
  })
})
