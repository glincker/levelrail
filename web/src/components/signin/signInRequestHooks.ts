import { useEffect, useState } from 'react'
import type { TFunction } from 'i18next'
import { parseDeviceLabel } from '../../lib/signInRequest'

export function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [active])
  return now
}

export function deviceTitle(label: string, t: TFunction<'signIn'>): string {
  const info = parseDeviceLabel(label)
  if (info.kind === 'cli') return t('request.deviceCli')
  if (info.kind === 'mcp') return t('request.deviceMcp')
  if (info.kind === 'unknown') return info.os || t('request.deviceUnknown')
  return info.os ? `${info.browser}, ${info.os}` : info.browser
}
