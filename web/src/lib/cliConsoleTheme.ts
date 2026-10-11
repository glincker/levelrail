export const DARK_THEME = {
  background: '#09090b',
  foreground: '#e4e4e7',
  cursor: '#e4e4e7',
  selectionBackground: '#3f3f46',
}

export const LIGHT_THEME = {
  background: '#fafafa',
  foreground: '#18181b',
  cursor: '#18181b',
  selectionBackground: '#d4d4d8',
}

export const RED = '\x1b[31m'
export const DIM = '\x1b[2m'
export const GREEN = '\x1b[32m'
export const RESET = '\x1b[0m'

export function isDark(): boolean {
  return document.documentElement.classList.contains('dark')
}
