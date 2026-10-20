let preferredTimeZone: string | undefined

/** setPreferredTimeZone applies the signed-in user's timezone preference to every absolute date shown. */
export function setPreferredTimeZone(tz: string | undefined): void {
  preferredTimeZone = tz && tz !== '' ? tz : undefined
}

/** formatAbsolute renders an instant in the preferred timezone, falling back to the browser's. */
export function formatAbsolute(at: string | Date): string {
  const date = at instanceof Date ? at : new Date(at)
  if (Number.isNaN(date.getTime())) return ''
  try {
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: preferredTimeZone,
    }).format(date)
  } catch {
    return date.toLocaleString()
  }
}
