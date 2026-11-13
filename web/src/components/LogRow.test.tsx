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
  wrap?: boolean,
  at?: number,
  now?: number,
) {
  const onToggle = vi.fn()
  render(
    <I18nextProvider i18n={testI18n}>
      <LogRow
        logLine={
          at === undefined
            ? { id: 1, line, stream }
            : { id: 1, line, stream, at }
        }
        index={0}
        start={0}
        expanded={expanded}
        onToggle={onToggle}
        measure={() => undefined}
        wrap={wrap}
        now={now}
      />
    </I18nextProvider>,
  )
  return { onToggle }
}

describe('LogRow', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows a level tag and keeps the line text in a plain selectable cell', () => {
    renderRow('WARN disk low', false)
    expect(screen.getByText('WRN')).toBeInTheDocument()
    const cell = screen.getByText('WARN disk low')
    expect(cell).toHaveClass('select-text')
    expect(cell.closest('button')).toBeNull()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('has no per-line copy control', () => {
    renderRow('{"level":"error","msg":"boom"}', true)
    expect(screen.queryByRole('button', { name: /copy/i })).toBeNull()
    expect(screen.queryByText('Copy line')).toBeNull()
  })

  it('toggles a JSON line from the gutter chevron only', async () => {
    const { onToggle } = renderRow('{"level":"error","msg":"boom"}', false)
    const user = userEvent.setup()
    await user.click(screen.getByText(/"msg":"boom"/))
    expect(onToggle).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Expand JSON' }))
    expect(onToggle).toHaveBeenCalledOnce()
  })

  it('pretty prints JSON when expanded', () => {
    renderRow('{"level":"error","msg":"boom"}', true)
    expect(
      screen.getByText((c) => c.includes('"msg": "boom"')),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Collapse JSON' }),
    ).toHaveAttribute('aria-expanded', 'true')
  })

  it('offers no expand control for plain text lines', () => {
    renderRow('plain text line', false)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
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

  it('wraps long lines by default and never truncates them', () => {
    const long = `{"msg":"${'x'.repeat(400)}"}`
    renderRow(long, false)
    const cell = screen.getByText(long)
    expect(cell).toHaveClass('whitespace-pre-wrap', 'break-words')
    expect(cell).not.toHaveClass('truncate')
  })

  it('keeps one line with sideways scroll when wrap is off', () => {
    renderRow('plain', false, 'stdout', false)
    const cell = screen.getByText('plain')
    expect(cell).toHaveClass('whitespace-pre')
    expect(cell).not.toHaveClass('truncate', 'whitespace-pre-wrap')
  })

  it('renders the level as text with an accessible name', () => {
    renderRow('ERROR boom', false)
    expect(screen.getByText('ERR')).toBeInTheDocument()
    expect(
      screen.getByText('error', { selector: '.sr-only' }),
    ).toBeInTheDocument()
  })

  it('shows a relative time with the absolute time in the title', () => {
    const at = Date.UTC(2026, 10, 12, 12, 0, 0)
    renderRow('hello', false, 'stdout', true, at, at + 12_000)
    const time = screen.getByText(/12/, { selector: 'time' })
    expect(time).toHaveAttribute('datetime', '2026-11-12T12:00:00.000Z')
    expect(time.getAttribute('title')).toMatch(/2026/)
    expect(time).toHaveClass('select-none')
  })

  it('shows no time when the line has none', () => {
    renderRow('hello', false, 'stdout', true, undefined, Date.now())
    expect(document.querySelector('time')).toBeNull()
  })
})
