import type { AttentionItem } from './attention'

export function greetingFor(hour: number): string {
  if (hour < 5) return 'Good evening'
  if (hour < 12) return 'Good morning'
  if (hour < 18) return 'Good afternoon'
  return 'Good evening'
}

export function platformPill(items: AttentionItem[]): {
  tone: 'success' | 'warning' | 'danger'
  label: string
} {
  const critical = items.filter((i) => i.severity === 'critical').length
  if (critical > 0) {
    return {
      tone: 'danger',
      label: `${critical} need${critical === 1 ? 's' : ''} attention`,
    }
  }
  if (items.length > 0) {
    return {
      tone: 'warning',
      label: `${items.length} warning${items.length === 1 ? '' : 's'}`,
    }
  }
  return { tone: 'success', label: 'All systems healthy' }
}
