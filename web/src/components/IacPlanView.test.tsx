import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import settingsEn from '../locales/en/settings.json'
import { IacPlanView, IacResultView } from './IacPlanView'
import type { IacPlan } from '../queries/iac'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

const plan: IacPlan = {
  hash: 'abc',
  summary: { create: 0, update: 1, delete: 0, noop: 1, error: 0 },
  changes: [
    {
      kind: 'App',
      name: 'web',
      action: 'update',
      file: 'infra/web.yaml',
      line: 3,
      fields: [
        { path: 'image', op: 'change', old: 'a:1', new: 'a:2' },
        { path: 'env.LOG', op: 'add', new: '(hidden)' },
      ],
    },
    { kind: 'Tag', name: 'x', action: 'noop' },
  ],
}

describe('IacPlanView', () => {
  it('shows the summary and field diffs, and hides unchanged items', () => {
    render(<IacPlanView plan={plan} />)
    expect(
      screen.getByText('0 to create, 1 to update, 0 to delete, 1 unchanged'),
    ).toBeTruthy()
    expect(screen.getByText('App/web')).toBeTruthy()
    expect(screen.getByText(/~ image: a:1 -> a:2/)).toBeTruthy()
    expect(screen.getByText(/\+ env\.LOG: \(hidden\)/)).toBeTruthy()
    expect(screen.queryByText('Tag/x')).toBeNull()
  })

  it('says so when nothing changes', () => {
    render(
      <IacPlanView
        plan={{
          ...plan,
          summary: { create: 0, update: 0, delete: 0, noop: 2, error: 0 },
          changes: [{ kind: 'Tag', name: 'x', action: 'noop' }],
        }}
      />,
    )
    expect(screen.getByText(/already matches/)).toBeTruthy()
  })

  it('renders errors, denied and warnings in English', () => {
    render(
      <IacPlanView
        plan={{
          ...plan,
          summary: { create: 0, update: 1, delete: 0, noop: 0, error: 2 },
          changes: [
            {
              kind: 'App',
              name: 'web',
              action: 'update',
              denied: true,
              warnings: ['careful'],
              fields: [],
              kept: [{ path: 'env.X', op: 'keep' }],
            },
          ],
        }}
      />,
    )
    expect(screen.getByText(/, 2 with errors/)).toBeTruthy()
    expect(screen.getByText('denied')).toBeTruthy()
    expect(screen.getByText('warning: careful')).toBeTruthy()
    expect(screen.getByText(/= env\.X \(kept, not in the files\)/)).toBeTruthy()
  })
})

describe('IacResultView', () => {
  it('lists denied and applied items', () => {
    render(
      <IacResultView
        results={[
          { kind: 'App', name: 'web', action: 'update', status: 'applied' },
          {
            kind: 'App',
            name: 'victim',
            action: 'update',
            status: 'denied',
            error: '403 forbidden',
          },
        ]}
      />,
    )
    expect(screen.getByText('denied')).toBeTruthy()
    expect(screen.getByText('App/victim')).toBeTruthy()
    expect(screen.getByText(/403 forbidden/)).toBeTruthy()
  })
})
