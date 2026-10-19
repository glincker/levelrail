import type { ReactNode } from 'react'
import {
  DeviceMobileIcon,
  FingerprintIcon,
  KeyIcon,
  LockKeyIcon,
  UsbIcon,
} from '@phosphor-icons/react/dist/ssr'
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
  if (providerKey === 'yubikey') return <UsbIcon className={cls} />
  if (providerKey === 'samsung') return <DeviceMobileIcon className={cls} />
  if (MANAGERS.has(providerKey)) return <LockKeyIcon className={cls} />
  if (providerKey === 'unknown') return <FingerprintIcon className={cls} />
  return <KeyIcon className={cls} />
}
