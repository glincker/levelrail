import type { Icon } from '@phosphor-icons/react'
import { PackageIcon } from '@phosphor-icons/react/dist/ssr'
import { CATEGORY_ICONS } from '../components/ServiceTemplateGrid'

// Literal class strings so Tailwind's scanner keeps them.
const TONES = {
  info: 'text-tone-info',
  success: 'text-tone-success',
  warning: 'text-tone-warning',
  accent: 'text-tone-accent',
} as const

const CATEGORY_TONE: Record<string, keyof typeof TONES> = {
  AI: 'accent',
  Analytics: 'info',
  Monitoring: 'info',
  Media: 'warning',
  Finance: 'success',
  Security: 'success',
  'Starter Kits': 'accent',
  Productivity: 'info',
}

export function categoryStyle(category: string): {
  icon: Icon
  toneClass: string
} {
  return {
    icon: CATEGORY_ICONS[category] ?? PackageIcon,
    toneClass: TONES[CATEGORY_TONE[category] ?? 'info'],
  }
}
