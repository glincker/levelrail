import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { TemplateFileField } from './TemplateFileField'
import templatesEn from '../../locales/en/templates.json'
import type { TemplateEnvPrompt } from '../../types/compose'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['templates'],
  defaultNS: 'templates',
  resources: { en: { templates: templatesEn } },
  interpolation: { escapeValue: false },
})

const prompt: TemplateEnvPrompt = {
  key: 'APP_CONF',
  label: 'App config',
  type: 'file',
  service: 'app',
  path: '/etc/app.conf',
  mode: '0444',
  default: 'listen=80',
}

function wrap(p: TemplateEnvPrompt, extra: { sizeError?: string } = {}) {
  const onChange = vi.fn()
  render(
    <I18nextProvider i18n={testI18n}>
      <TemplateFileField prompt={p} value="" onChange={onChange} {...extra} />
    </I18nextProvider>,
  )
  return onChange
}

describe('TemplateFileField', () => {
  it('shows the default body with mount and mode hints', () => {
    wrap(prompt)
    const box = screen.getByRole<HTMLTextAreaElement>('textbox')
    expect(box.value).toBe('listen=80')
    expect(screen.getByText(/app at \/etc\/app.conf/)).toBeTruthy()
    expect(screen.getByText('Mode 0444')).toBeTruthy()
    expect(screen.queryByText('Secret file')).toBeNull()
  })

  it('marks a secret file and masks it until focused', async () => {
    wrap({ ...prompt, secret: true })
    const box = screen.getByRole('textbox')
    expect(screen.getByText('Secret file')).toBeTruthy()
    expect(box.className).toContain('text-security')
    await userEvent.click(box)
    expect(box.className).not.toContain('text-security')
  })

  it('reports edits and shows the API size message', async () => {
    const onChange = wrap(prompt, {
      sizeError: 'setting APP_CONF is 9 bytes, over the 4 byte limit',
    })
    await userEvent.type(screen.getByRole('textbox'), 'x')
    expect(onChange).toHaveBeenCalled()
    expect(screen.getByText(/over the 4 byte limit/)).toBeTruthy()
  })
})
