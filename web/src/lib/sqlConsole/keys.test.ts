import { describe, expect, it } from 'vitest'
import { editorKeyAction, type KeyInput } from './keys'

const k = (key: string, o: Partial<KeyInput> = {}): KeyInput => ({
  key,
  ctrlKey: false,
  metaKey: false,
  shiftKey: false,
  altKey: false,
  popupOpen: false,
  ...o,
})

describe('editorKeyAction', () => {
  it('runs on Cmd or Ctrl+Enter even with the popup open', () => {
    expect(
      editorKeyAction(k('Enter', { metaKey: true, popupOpen: true })),
    ).toBe('run')
    expect(
      editorKeyAction(k('Enter', { ctrlKey: true, popupOpen: true })),
    ).toBe('run')
  })

  it('accepts only on plain Enter or Tab while open', () => {
    expect(editorKeyAction(k('Enter', { popupOpen: true }))).toBe('accept')
    expect(editorKeyAction(k('Tab', { popupOpen: true }))).toBe('accept')
    expect(
      editorKeyAction(k('Enter', { shiftKey: true, popupOpen: true })),
    ).toBe('none')
    expect(editorKeyAction(k('Enter'))).toBe('none')
  })

  it('closes, moves and opens', () => {
    expect(editorKeyAction(k('Escape', { popupOpen: true }))).toBe('close')
    expect(editorKeyAction(k('ArrowDown', { popupOpen: true }))).toBe('next')
    expect(editorKeyAction(k('ArrowUp', { popupOpen: true }))).toBe('prev')
    expect(editorKeyAction(k('ArrowDown'))).toBe('none')
    expect(editorKeyAction(k(' ', { ctrlKey: true }))).toBe('open')
  })
})
