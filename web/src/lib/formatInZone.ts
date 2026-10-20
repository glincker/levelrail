// formatInZone renders an instant in the server's time zone, so a schedule
// reads the same for every operator whatever their browser's zone is.
export function formatInZone(iso: string, tz: string): string {
  try {
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: tz,
    }).format(new Date(iso))
  } catch {
    return new Date(iso).toLocaleString()
  }
}
