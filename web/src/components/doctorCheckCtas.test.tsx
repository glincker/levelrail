import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import { getCheckCta } from './doctorCheckCtas'
import fixitEn from '../locales/en/fixit.json'
import type { DoctorCheck } from '../queries/systemDoctor'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['fixit'],
  defaultNS: 'fixit',
  resources: { en: { fixit: fixitEn } },
  interpolation: { escapeValue: false },
})

function check(over: Partial<DoctorCheck>): DoctorCheck {
  return {
    code: 'container_runtime',
    name: 'Container runtime',
    status: 'warn',
    message: 'm',
    ...over,
  }
}

describe('container_runtime CTA', () => {
  it('shows engine kind and support level, and points at the socket setting', () => {
    const cta = getCheckCta(
      check({ engine_kind: 'colima', support_level: 'best-effort' }),
    )
    expect(cta?.message).toContain('APP_CONTAINER_RUNTIME_SOCKET')
    render(<I18nextProvider i18n={testI18n}>{cta?.action}</I18nextProvider>)
    expect(screen.getByText(/Colima/)).toBeInTheDocument()
    expect(screen.getByText(/Best effort/)).toBeInTheDocument()
  })

  it('labels an unverified unknown engine', () => {
    const cta = getCheckCta(
      check({ engine_kind: 'unknown', support_level: 'unverified' }),
    )
    render(<I18nextProvider i18n={testI18n}>{cta?.action}</I18nextProvider>)
    expect(screen.getByText(/Unknown engine/)).toBeInTheDocument()
    expect(screen.getByText(/Unverified/)).toBeInTheDocument()
  })

  it('shows nothing for an ok check without runtime data', () => {
    expect(getCheckCta(check({ status: 'ok' }))).toBeNull()
  })
})
