import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ExposureRestrictDialog } from './ExposureRestrictDialog'
import exposureEn from '../../locales/en/exposure.json'
import type { ExposureFinding } from '../../types/exposure'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['exposure'],
  defaultNS: 'exposure',
  resources: { en: { exposure: exposureEn } },
  interpolation: { escapeValue: false },
})

const finding: ExposureFinding = {
  container: 'db',
  image: 'postgres:16',
  owner: { kind: 'unmanaged' },
  image_kind: 'datastore',
  protocol: 'tcp',
  host_port: 2222,
  container_port: 2222,
  binds: ['0.0.0.0'],
  class: 'exposed',
  severity: 'high',
  managed: false,
  explanation: 'exposed',
  can_restrict: true,
  outside_check: { status: 'not_run' },
}

afterEach(() => vi.unstubAllGlobals())

describe('ExposureRestrictDialog', () => {
  it('blocks apply on a lockout risk until it is accepted, and sends auto_revert', async () => {
    const bodies: Record<string, unknown>[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.body)
          bodies.push(
            JSON.parse(init.body as string) as Record<string, unknown>,
          )
        const body = url.endsWith('/preview')
          ? {
              commands: ['iptables -w -I DOCKER-USER 1 ...'],
              drops: 'drops',
              tag: 't',
              persistence: 'p',
              port: 2222,
              protocol: 'tcp',
              allow: ['10.0.0.2/32'],
              warnings: [],
              applied: false,
              lockout: ['port 2222 is where sshd listens'],
              caller_warning: 'you are connected from this host (127.0.0.1)',
            }
          : { applied: true, pending: true, revert_at: '2026-01-01T00:01:00Z' }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve(body),
        } as Response)
      }),
    )
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <I18nextProvider i18n={testI18n}>
          <ExposureRestrictDialog
            finding={finding}
            open
            onOpenChange={() => {}}
          />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    await userEvent.type(screen.getByRole('textbox'), '10.0.0.2')
    await userEvent.click(screen.getByRole('button', { name: 'Preview rules' }))
    expect(
      await screen.findByText('port 2222 is where sshd listens'),
    ).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('checkbox', { name: /I understand this traffic/ }),
    )
    expect(screen.getByText(/connected from this host/)).toBeInTheDocument()
    const apply = screen.getByRole('button', { name: 'Apply restriction' })
    expect(apply).toBeDisabled()
    await userEvent.click(
      screen.getByRole('checkbox', { name: /accept the risk/ }),
    )
    expect(apply).toBeEnabled()
    await userEvent.click(apply)
    expect(
      await screen.findByText('Restriction applied on trial'),
    ).toBeInTheDocument()
    const put = bodies.find((b) => b.confirm === true)
    expect(put?.auto_revert).toBe(true)
    expect(put?.force_lockout).toBe(true)
  })
})
