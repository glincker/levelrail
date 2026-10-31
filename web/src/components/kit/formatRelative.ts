import i18n from '../../i18n'

// Non-English locales use Intl so units and word order follow the language.
function localized(lang: string, seconds: number): string {
  const rtf = new Intl.RelativeTimeFormat(lang, {
    numeric: 'auto',
    style: 'short',
  })
  const abs = Math.abs(seconds)
  const sign = seconds < 0 ? -1 : 1
  if (abs < 45) return rtf.format(0, 'second')
  if (abs < 3600)
    return rtf.format(sign * Math.max(1, Math.round(abs / 60)), 'minute')
  if (abs < 86400) return rtf.format(sign * Math.round(abs / 3600), 'hour')
  if (abs < 86400 * 30) return rtf.format(sign * Math.round(abs / 86400), 'day')
  if (abs < 86400 * 365)
    return rtf.format(sign * Math.round(abs / (86400 * 30)), 'month')
  return rtf.format(sign * Math.round(abs / (86400 * 365)), 'year')
}

export function formatRelative(
  at: string | Date,
  now: number = Date.now(),
): string {
  const ms = at instanceof Date ? at.getTime() : Date.parse(at)
  if (!Number.isFinite(ms)) return ''
  const diff = Math.round((now - ms) / 1000)
  const lang = i18n.language
  if (lang && !lang.startsWith('en')) return localized(lang, -diff)
  if (diff < 0) {
    const ahead = -diff
    if (ahead < 45) return 'in a moment'
    if (ahead < 3600) return `in ${Math.round(ahead / 60)}m`
    if (ahead < 86400) return `in ${Math.round(ahead / 3600)}h`
    return `in ${Math.round(ahead / 86400)}d`
  }
  if (diff < 45) return 'just now'
  if (diff < 3600) return `${Math.max(1, Math.round(diff / 60))}m ago`
  if (diff < 86400) return `${Math.round(diff / 3600)}h ago`
  if (diff < 86400 * 30) return `${Math.round(diff / 86400)}d ago`
  if (diff < 86400 * 365) return `${Math.round(diff / (86400 * 30))}mo ago`
  return `${Math.round(diff / (86400 * 365))}y ago`
}
