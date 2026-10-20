import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import gitBuildEn from '../locales/en/gitBuild.json'
import { StaticBuildFields } from './StaticBuildFields'
import { staticBuildMismatch, type StaticBuildValue } from '../lib/staticBuild'
import type { StaticBuildSuggestion } from '../types/gitSource'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['gitBuild'],
  defaultNS: 'gitBuild',
  resources: { en: { gitBuild: gitBuildEn } },
  interpolation: { escapeValue: false },
})

const vite: StaticBuildSuggestion = {
  preset_id: 'vite',
  preset_name: 'React / Vite',
  command: 'npm run build',
  publish_dir: 'dist',
}

function renderFields(
  value: StaticBuildValue,
  suggestion: StaticBuildSuggestion | undefined,
  onChange = vi.fn(),
) {
  render(
    <I18nextProvider i18n={testI18n}>
      <StaticBuildFields
        value={value}
        suggestion={suggestion}
        disabled={false}
        onChange={onChange}
      />
    </I18nextProvider>,
  )
  return onChange
}

describe('staticBuildMismatch', () => {
  it.each([
    [{ command: '', publish: '' }, false],
    [{ command: 'npm run build', publish: 'dist' }, false],
    [{ command: 'npm run build', publish: '' }, true],
    [{ command: '', publish: 'dist' }, true],
    [{ command: '  ', publish: 'dist' }, true],
  ])('%j -> %s', (value, want) => {
    expect(staticBuildMismatch(value)).toBe(want)
  })
})

describe('StaticBuildFields', () => {
  it('offers the detected preset and applies it on request', async () => {
    const onChange = renderFields({ command: '', publish: '' }, vite)
    expect(screen.getByText('Detected React / Vite')).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'Use its build settings' }),
    )
    expect(onChange).toHaveBeenCalledWith({
      command: 'npm run build',
      publish: 'dist',
    })
  })

  it('hides the offer once the values match the preset', () => {
    renderFields({ command: 'npm run build', publish: 'dist' }, vite)
    expect(screen.queryByText('Detected React / Vite')).toBeNull()
  })

  it('lets the operator override and warns on a half-filled pair', async () => {
    const onChange = renderFields({ command: 'make', publish: '' }, undefined)
    expect(
      screen.getByText(
        'Set both the build command and the publish directory, or clear both.',
      ),
    ).toBeTruthy()
    await userEvent.type(screen.getByLabelText('Publish directory'), 'p')
    expect(onChange).toHaveBeenCalledWith({ command: 'make', publish: 'p' })
  })
})
