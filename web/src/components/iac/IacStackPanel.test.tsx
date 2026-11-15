import { fireEvent, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import settingsEn from '../../locales/en/settings.json'
import stacksEn from '../../locales/en/stacks.json'
import { IacStackPanel } from './IacStackPanel'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings', 'stacks'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn, stacks: stacksEn } },
  interpolation: { escapeValue: false },
})

const data = vi.hoisted(() => ({
  stacks: [
    {
      name: 'prod',
      latest_revision: 2,
      drift: {
        status: 'drifted',
        summary: [{ kind: 'App', name: 'web', action: 'vanished' }],
      },
    },
  ],
}))

vi.mock('../../queries/stacks', () => ({
  stacksQueryOptions: () => ({ queryKey: ['stacks'], select: 'stacks' }),
  stackHistoryQueryOptions: () => ({ queryKey: ['h'], select: 'history' }),
  stackDiffQueryOptions: () => ({ queryKey: ['d'], select: 'diff' }),
}))
vi.mock('@tanstack/react-query', () => ({
  useQuery: (o: { select: string }) => {
    if (o.select === 'stacks') return { data: data.stacks }
    if (o.select === 'history')
      return {
        data: {
          revisions: [
            {
              number: 2,
              status: 'applied',
              plan_hash: 'h',
              actor: 'ana',
              created_at: '2026-11-14T10:00:00Z',
            },
          ],
        },
      }
    return { data: { changes: [] } }
  },
}))

describe('IacStackPanel', () => {
  it('shows the timeline and drift for the preselected stack', () => {
    render(<IacStackPanel />)
    expect(screen.getByRole('list', { name: 'Revision timeline' })).toBeTruthy()
    expect(screen.getAllByText('Drifted (1)').length).toBeGreaterThan(0)
    // An unknown drift action falls back to the raw value.
    fireEvent.click(screen.getByText(/Drift: 1/))
    expect(screen.getByText('vanished')).toBeTruthy()
    expect(
      screen.getByRole('button', { name: 'Compare with live state' }),
    ).toBeTruthy()
  })
})
