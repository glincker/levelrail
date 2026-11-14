import { fireEvent, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import settingsEn from '../../locales/en/settings.json'
import stacksEn from '../../locales/en/stacks.json'
import type { StackRevisionSummary } from '../../types/stacks'
import { IacRevisionTimeline } from './IacRevisionTimeline'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings', 'stacks'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn, stacks: stacksEn } },
  interpolation: { escapeValue: false },
})

function rev(n: number, status: string): StackRevisionSummary {
  return {
    number: n,
    // A status this build does not know must not crash the timeline.
    status: status as StackRevisionSummary['status'],
    plan_hash: 'h',
    actor: 'ana',
    created_at: '2026-11-14T10:00:00Z',
  }
}

describe('IacRevisionTimeline', () => {
  it('selects a revision and survives an unknown status', () => {
    const onSelect = vi.fn()
    render(
      <IacRevisionTimeline
        revisions={[rev(2, 'applied'), rev(1, 'from_the_future')]}
        selected={2}
        onSelect={onSelect}
      />,
    )
    const buttons = screen.getAllByRole('button')
    expect(buttons[0]?.getAttribute('aria-pressed')).toBe('true')
    fireEvent.click(buttons[1] as HTMLElement)
    expect(onSelect).toHaveBeenCalledWith(1)
  })

  it('shows the empty note', () => {
    render(
      <IacRevisionTimeline revisions={[]} selected={null} onSelect={vi.fn()} />,
    )
    expect(screen.getByText('No revisions yet.')).toBeTruthy()
  })
})
