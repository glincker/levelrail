import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import { PublishedPortsField } from './PublishedPortsField'
import httpsEn from '../locales/en/https.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['https'],
  defaultNS: 'https',
  resources: { en: { https: httpsEn } },
  interpolation: { escapeValue: false },
})

function renderField(
  ports?: Parameters<typeof PublishedPortsField>[0]['ports'],
) {
  return render(
    <I18nextProvider i18n={testI18n}>
      <PublishedPortsField ports={ports} />
    </I18nextProvider>,
  )
}

describe('PublishedPortsField', () => {
  it('renders nothing without ports', () => {
    const { container } = renderField(undefined)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows protocol badges and the container port when it differs', () => {
    renderField([
      { container_port: 53, host_port: 5353, protocol: 'udp' },
      { container_port: 53, host_port: 53, protocol: 'tcp' },
    ])
    expect(screen.getByText('UDP')).toBeInTheDocument()
    expect(screen.getByText('TCP')).toBeInTheDocument()
    expect(screen.getByText('5353 \u2192 53')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
  })
})
