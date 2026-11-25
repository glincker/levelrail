import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import databasesEn from '../../../locales/en/databases.json'
import { JsonTree } from './JsonTree'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['databases'],
  defaultNS: 'databases',
  resources: { en: { databases: databasesEn } },
  interpolation: { escapeValue: false },
})

const DOC = {
  _id: { $oid: 'abc123' },
  name: 'Ada',
  address: { city: 'Paris' },
  tags: ['vip'],
}

function renderTree(signal?: { version: number; open: boolean }) {
  return render(
    <I18nextProvider i18n={testI18n}>
      <JsonTree value={DOC} signal={signal} />
    </I18nextProvider>,
  )
}

describe('JsonTree', () => {
  it('shows the top level open and nested objects collapsed', () => {
    renderTree()
    expect(screen.getByText(/ObjectId/)).toBeTruthy()
    expect(screen.getByText('"Ada"')).toBeTruthy()
    expect(screen.queryByText('"Paris"')).toBeNull()
  })
  it('opens a nested node on click', () => {
    renderTree()
    fireEvent.click(screen.getByRole('button', { name: /address/ }))
    expect(screen.getByText('"Paris"')).toBeTruthy()
  })
  it('expands everything from the expand signal', () => {
    renderTree({ version: 1, open: true })
    expect(screen.getByText('"Paris"')).toBeTruthy()
    expect(screen.getByText('"vip"')).toBeTruthy()
  })
})
