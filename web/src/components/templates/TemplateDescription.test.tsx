import { render, screen } from '@testing-library/react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { createInstance } from 'i18next'
import { describe, expect, it } from 'vitest'
import templates from '../../locales/en/templates.json'
import { TemplateDescription } from './TemplateDescription'

async function renderDescription(description?: string, slogan?: string) {
  const i18n = createInstance()
  await i18n
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { templates } } })
  return render(
    <I18nextProvider i18n={i18n}>
      <TemplateDescription description={description} slogan={slogan} />
    </I18nextProvider>,
  )
}

describe('TemplateDescription', () => {
  it('renders notes as text with the label', async () => {
    await renderDescription('Open TCP 21115.\n<b>x</b>', 'Short')
    expect(screen.getByText('Setup notes')).toBeTruthy()
    const body = screen.getByText(/Open TCP 21115/)
    expect(body.textContent).toContain('<b>x</b>')
    expect(body.querySelector('b')).toBeNull()
  })

  it('renders nothing without a description or when it repeats the slogan', async () => {
    const { container } = await renderDescription(undefined, 'Short')
    expect(container.firstChild).toBeNull()
    const again = await renderDescription('Short', 'Short')
    expect(again.container.firstChild).toBeNull()
  })
})

describe('TemplateDescription long text', () => {
  it('wraps a 200 character unbroken token inside its box', async () => {
    const token = 'x'.repeat(200)
    await renderDescription(`Visit https://example.com/${token}`)
    const body = screen.getByText(/Visit https/)
    const box = body.parentElement
    expect(body.className).toContain('whitespace-pre-line')
    expect(box?.className).toContain('min-w-0')
    expect(box?.className).toContain('break-words')
    expect(box?.className).toContain('[overflow-wrap:anywhere]')
  })
})
