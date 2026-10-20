import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { LogRow } from './LogRow'
import commonEn from '../locales/en/common.json'
import type { LogLine } from '../hooks/useLogStream'

// Real JSON content (not a stub), matching AutoRollbackCard.test.tsx's own
// approach: LogRow's 'system' row renders through useTranslation('common').
const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common'],
  defaultNS: 'common',
  resources: { en: { common: commonEn } },
  interpolation: { escapeValue: false },
})

function renderRow(
  line: string,
  expanded: boolean,
  stream: LogLine['stream'] = 'stdout',
) {
  const onToggle = vi.fn()
  render(
    <I18nextProvider i18n={testI18n}>
      <LogRow
        logLine={{ id: 1, line, stream }}
        index={0}
        start={0}
        expanded={expanded}
        onToggle={onToggle}
        measure={() => undefined}
      />
    </I18nextProvider>,
  )
  return { onToggle }
}

describe('LogRow', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows a level tag and toggles on click', async () => {
    const { onToggle } = renderRow('WARN disk low', false)
    expect(screen.getByText('WRN')).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button'))
    expect(onToggle).toHaveBeenCalledOnce()
  })

  it('pretty prints JSON when expanded', () => {
    renderRow('{"level":"error","msg":"boom"}', true)
    expect(
      screen.getByText((c) => c.includes('"msg": "boom"')),
    ).toBeInTheDocument()
    expect(screen.getByText('Copy line')).toBeInTheDocument()
  })

  it('shows non-JSON lines in full when expanded', () => {
    renderRow('plain text line', true)
    expect(screen.getAllByText('plain text line')).toHaveLength(2)
  })

  it('renders a system line as a translated marker, not raw wire text', () => {
    renderRow('previous container instance ended', false, 'system')
    expect(
      screen.getByText('Previous container instance ended'),
    ).toBeInTheDocument()
    expect(
      screen.queryByText('previous container instance ended'),
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})
