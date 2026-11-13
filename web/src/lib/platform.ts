interface NavigatorWithHints {
  userAgentData?: { platform?: string }
  platform?: string
}

/** True on Apple platforms. Reads the modern hint first and falls back to the legacy field. */
export function isMacPlatform(nav?: NavigatorWithHints): boolean {
  const n: NavigatorWithHints | undefined =
    nav ?? (typeof navigator === 'undefined' ? undefined : navigator)
  const raw = n?.userAgentData?.platform || n?.platform || ''
  return /mac|iphone|ipad|ipod/i.test(raw)
}

export type ModifierName = 'alt' | 'mod'

/** Display label for a modifier: Option and Command on Mac, Alt and Ctrl elsewhere. */
export function modifierLabel(
  name: ModifierName,
  mac = isMacPlatform(),
): string {
  if (name === 'alt') return mac ? '⌥' : 'Alt'
  return mac ? '⌘' : 'Ctrl'
}

/** A shortcut as shown to the user, e.g. "⌥1" on Mac and "Alt+1" elsewhere. */
export function shortcutLabel(
  name: ModifierName,
  key: string,
  mac = isMacPlatform(),
): string {
  return mac
    ? `${modifierLabel(name, true)}${key}`
    : `${modifierLabel(name, false)}+${key}`
}
