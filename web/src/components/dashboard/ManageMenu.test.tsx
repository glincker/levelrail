import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import dashboardEn from '../../locales/en/dashboard.json'
import { ManageMenu, type ManageMenuProps } from './ManageMenu'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['dashboard'],
  defaultNS: 'dashboard',
  resources: { en: { dashboard: dashboardEn } },
  interpolation: { escapeValue: false },
})

function setup(over: Partial<ManageMenuProps> = {}) {
  const props: ManageMenuProps = {
    board: { editable: true },
    editing: false,
    busy: false,
    onNew: vi.fn(),
    onStarter: vi.fn(),
    onImportFile: vi.fn(),
    onEdit: vi.fn(),
    onRename: vi.fn(),
    onExport: vi.fn(),
    onDelete: vi.fn(),
    ...over,
  }
  render(
    <I18nextProvider i18n={i18n}>
      <ManageMenu {...props} />
    </I18nextProvider>,
  )
  return props
}

async function open(name: string) {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Manage' }))
  await user.click(await screen.findByRole('menuitem', { name }))
}

describe('ManageMenu', () => {
  it.each([
    ['New board', 'onNew'],
    ['New from a starter', 'onStarter'],
    ['Edit board', 'onEdit'],
    ['Rename', 'onRename'],
    ['Export JSON', 'onExport'],
    ['Delete', 'onDelete'],
  ] as const)('%s calls %s', async (label, handler) => {
    const props = setup()
    await open(label)
    expect(props[handler]).toHaveBeenCalledOnce()
  })

  it('offers only creation and import on Start and Metrics', async () => {
    setup({ board: null })
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Manage' }))
    expect(
      await screen.findByRole('menuitem', { name: 'New board' }),
    ).toBeTruthy()
    expect(screen.queryByRole('menuitem', { name: 'Delete' })).toBeNull()
    expect(screen.queryByRole('menuitem', { name: 'Export JSON' })).toBeNull()
  })

  it('hides edit, rename and delete on a read-only board', async () => {
    setup({ board: { editable: false } })
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Manage' }))
    expect(
      await screen.findByRole('menuitem', { name: 'Export JSON' }),
    ).toBeTruthy()
    expect(screen.queryByRole('menuitem', { name: 'Rename' })).toBeNull()
    expect(screen.queryByRole('menuitem', { name: 'Delete' })).toBeNull()
  })

  it('passes a chosen JSON file to onImportFile', async () => {
    const props = setup()
    const user = userEvent.setup()
    const file = new File(['{}'], 'board.json', { type: 'application/json' })
    await user.upload(screen.getByLabelText('Dashboard JSON file'), file)
    expect(props.onImportFile).toHaveBeenCalledWith(file)
  })
})
