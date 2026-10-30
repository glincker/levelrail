import type { ReactNode } from 'react'
import {
  FingerprintPatternGlassIcon,
  KeyGlassIcon,
  LockGlassIcon,
  SmartphoneGlassIcon,
  UsbGlassIcon,
} from '@thesvg/ui-react'
import { BrandIcon, type BrandIconName } from '@/components/BrandIcon'

const BRAND_BY_PROVIDER: Record<string, BrandIconName> = {
  apple: 'apple',
  google: 'google',
  windows: 'microsoft',
  chrome: 'chrome',
  edge: 'edge',
}

const MANAGERS = new Set([
  '1password',
  'bitwarden',
  'dashlane',
  'keeper',
  'nordpass',
  'enpass',
])

export function PasskeyProviderIcon({
  providerKey,
}: {
  providerKey: string
}): ReactNode {
  const brand = BRAND_BY_PROVIDER[providerKey]
  if (brand) {
    return <BrandIcon name={brand} className="size-5" />
  }
  const cls = 'size-5 text-muted-foreground'
  if (providerKey === 'yubikey')
    return <UsbGlassIcon size="1.4em" className={cls} />
  if (providerKey === 'samsung')
    return <SmartphoneGlassIcon size="1.4em" className={cls} />
  if (MANAGERS.has(providerKey))
    return <LockGlassIcon size="1.4em" className={cls} />
  if (providerKey === 'unknown')
    return <FingerprintPatternGlassIcon size="1.4em" className={cls} />
  return <KeyGlassIcon size="1.4em" className={cls} />
}
