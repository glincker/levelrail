import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18n from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import en from '../../../locales/en/databases.json'
import { BranchesPlaceholder } from './BranchesPlaceholder'
import { IntegrationsPlaceholder } from './IntegrationsPlaceholder'
import { SettingsShell } from './SettingsShell'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock('../../../queries/databases', () => ({
  useDatabase: () => ({
    data: { name: 'main', engine: 'postgres', version: '16' },
  }),
}))
vi.mock('../../MoveToProjectDialog', () => ({
  MoveToProjectDialog: () => null,
}))
vi.mock('../../ResourceLabelsCard', () => ({ ResourceLabelsCard: () => null }))
vi.mock('../../DeleteDatabaseDialog', () => ({
  DeleteDatabaseDialog: () => null,
}))

const RAW_KEY = /^[a-z]+(\.[a-zA-Z]+){2,}$/

async function setup() {
  const inst = i18n.createInstance()
  await inst.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { databases: en } },
    ns: ['databases'],
    defaultNS: 'databases',
  })
  return inst
}

function texts(root: HTMLElement) {
  const out: string[] = []
  const w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  for (let n = w.nextNode(); n; n = w.nextNode()) {
    const v = n.textContent?.trim()
    if (v) out.push(v)
  }
  return out
}

describe('database home pages', () => {
  it('render no raw i18n keys', async () => {
    const inst = await setup()
    const sections = ['general', 'danger'] as const
    for (const section of sections) {
      const { container, unmount } = render(
        <I18nextProvider i18n={inst}>
          <SettingsShell name="main" section={section} />
        </I18nextProvider>,
      )
      expect(texts(container).filter((s) => RAW_KEY.test(s))).toEqual([])
      unmount()
    }
    const { container } = render(
      <I18nextProvider i18n={inst}>
        <BranchesPlaceholder />
        <IntegrationsPlaceholder />
      </I18nextProvider>,
    )
    expect(texts(container).filter((s) => RAW_KEY.test(s))).toEqual([])
  })
})
