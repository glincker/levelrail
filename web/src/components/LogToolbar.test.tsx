import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import uiSharedEn from '../locales/en/uiShared.json'
import { LogToolbar } from './LogToolbar'
import type { LogFilter } from '../lib/logFilter'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['uiShared'],
  defaultNS: 'uiShared',
  resources: { en: { uiShared: uiSharedEn } },
  interpolation: { escapeValue: false },
})

function setup(
  filter: LogFilter = { text: '', stderrOnly: false },
  shown = 5,
  wrap = true,
) {
  const onWrapChange = vi.fn()
  const onFilterChange = vi.fn()
  const onCopySelection = vi.fn(() => Promise.resolve())
  const onCopyAll = vi.fn(() => Promise.resolve())
  const onDownload = vi.fn()
  const onJumpToError = vi.fn()
  render(
    <LogToolbar
      filter={filter}
      onFilterChange={onFilterChange}
      shown={shown}
      total={5}
      onCopySelection={onCopySelection}
      onCopyAll={onCopyAll}
      onDownload={onDownload}
      onJumpToError={onJumpToError}
      wrap={wrap}
      onWrapChange={onWrapChange}
    />,
  )
  return {
    onFilterChange,
    onCopySelection,
    onCopyAll,
    onDownload,
    onJumpToError,
    onWrapChange,
  }
}

describe('LogToolbar', () => {
  afterEach(() => {
    cleanup()
  })

  it('checks the active level segment', () => {
    setup({ text: '', stderrOnly: false, level: 'warnings' })
    expect(screen.getByRole('radio', { name: 'Warnings' })).toBeChecked()
    expect(screen.getByRole('radio', { name: 'All' })).not.toBeChecked()
    expect(
      screen.getByRole('radiogroup', { name: 'Log level' }),
    ).toBeInTheDocument()
  })

  it('changes the level and clears stderrOnly', async () => {
    const { onFilterChange } = setup({ text: 'x', stderrOnly: true })
    await userEvent.setup().click(screen.getByRole('radio', { name: 'Errors' }))
    expect(onFilterChange).toHaveBeenCalledWith({
      text: 'x',
      stderrOnly: false,
      level: 'errors',
    })
  })

  it('emits text filter changes', async () => {
    const { onFilterChange } = setup()
    await userEvent.setup().type(screen.getByLabelText('Filter log lines'), 'a')
    expect(onFilterChange).toHaveBeenCalledWith({
      text: 'a',
      stderrOnly: false,
    })
  })

  it('shows the filtered count in a status region', () => {
    setup(undefined, 2)
    expect(screen.getByRole('status')).toHaveTextContent('2 of 5 lines')
  })

  it('offers one action set: copy selection, copy all visible, download', async () => {
    const { onCopySelection, onCopyAll, onDownload } = setup()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Copy selection/ }))
    expect(onCopySelection).toHaveBeenCalledOnce()
    expect(onCopyAll).not.toHaveBeenCalled()
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Copied to clipboard',
    )
    await user.click(screen.getByRole('button', { name: /Copy all visible/ }))
    expect(onCopyAll).toHaveBeenCalledOnce()
    await user.click(screen.getByRole('button', { name: /Download/ }))
    expect(onDownload).toHaveBeenCalledOnce()
  })

  it('does not collapse the text selection when pressing copy selection', () => {
    setup()
    const btn = screen.getByRole('button', { name: /Copy selection/ })
    const notPrevented = fireEvent.mouseDown(btn)
    expect(notPrevented).toBe(false)
  })

  it('calls jump to error', async () => {
    const { onJumpToError } = setup()
    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: /Jump to first error/ }))
    expect(onJumpToError).toHaveBeenCalledOnce()
  })

  it('exposes wrap as a pressed toggle and flips it', async () => {
    const { onWrapChange } = setup(undefined, 5, true)
    const toggle = screen.getByRole('button', { name: 'Wrap lines' })
    expect(toggle).toHaveAttribute('aria-pressed', 'true')
    await userEvent.setup().click(toggle)
    expect(onWrapChange).toHaveBeenCalledWith(false)
  })
})
