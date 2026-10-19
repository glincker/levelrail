export type PasskeyDeviceKind =
  'mac' | 'iphone' | 'ipad' | 'windows' | 'android' | 'linux' | 'generic'

interface NavigatorWithUAData extends Navigator {
  userAgentData?: { platform?: string }
}

export function detectPasskeyDevice(
  nav: NavigatorWithUAData = navigator,
): PasskeyDeviceKind {
  const ua = nav.userAgent
  if (/iPhone/i.test(ua)) return 'iphone'
  if (/iPad/i.test(ua)) return 'ipad'
  const platform = (
    nav.userAgentData?.platform ??
    nav.platform ??
    ''
  ).toLowerCase()
  if (/android/i.test(ua) || platform === 'android') return 'android'
  if (platform.includes('mac') || /Macintosh/i.test(ua)) return 'mac'
  if (platform.includes('win') || /Windows/i.test(ua)) return 'windows'
  if (platform.includes('linux') || /Linux|X11/i.test(ua)) return 'linux'
  return 'generic'
}
