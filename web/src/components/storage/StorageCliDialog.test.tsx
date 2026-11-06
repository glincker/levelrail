import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import storageCliEn from '../../locales/en/storageCli.json'
import { StorageCliDialog } from './StorageCliDialog'
import {
  STORAGE_CLI_COMMANDS,
  commandFor,
  roleAllows,
} from './storageCliCommands'

vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => false }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'storageCli'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, storageCli: storageCliEn } },
  interpolation: { escapeValue: false },
})

async function open(props: { role?: 'viewer' | 'admin'; node?: string } = {}) {
  render(
    <I18nextProvider i18n={i18n}>
      <StorageCliDialog serverUrl="https://lr.example" {...props}>
        <button type="button">Terminal</button>
      </StorageCliDialog>
    </I18nextProvider>,
  )
  await userEvent.click(screen.getByRole('button', { name: 'Terminal' }))
}

describe('storage CLI catalog', () => {
  it('never lists system-prune and has a description for every entry', () => {
    for (const c of STORAGE_CLI_COMMANDS) {
      expect(c.command).not.toContain('system-prune')
      expect(storageCliEn.commands).toHaveProperty(c.description)
    }
  })

  it('personalises node-aware commands and ranks roles', () => {
    const gc = STORAGE_CLI_COMMANDS.find((c) => c.id === 'imagesGcPreview')
    expect(gc).toBeDefined()
    if (gc) {
      expect(commandFor(gc, 'n1')).toBe('levelrail-cli images gc --node n1')
    }
    expect(roleAllows('viewer', 'admin')).toBe(false)
    expect(roleAllows('admin', 'viewer')).toBe(true)
  })
})

describe('StorageCliDialog', () => {
  it('shows usage commands, the safety note and the real role', async () => {
    await open()
    expect(
      screen.getByRole('group', { name: 'Command: levelrail-cli doctor disk' }),
    ).toBeInTheDocument()
    expect(screen.getByText(/Your role: Viewer/)).toBeInTheDocument()
    expect(screen.getByText(/Dry run first/)).toBeInTheDocument()
  })

  it('dims admin commands for a viewer but keeps them copyable', async () => {
    await open({ role: 'viewer' })
    await userEvent.click(screen.getByRole('tab', { name: 'Clean safely' }))
    expect(screen.getAllByText('Needs admin').length).toBeGreaterThan(0)
    expect(screen.getByText(/system-prune is not listed/)).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /copy/i })[0]).toBeEnabled()
  })

  it('adds the node to image commands', async () => {
    await open({ role: 'admin', node: 'edge-1' })
    expect(
      screen.getByRole('group', {
        name: 'Command: levelrail-cli images list --node edge-1',
      }),
    ).toBeInTheDocument()
  })
})
