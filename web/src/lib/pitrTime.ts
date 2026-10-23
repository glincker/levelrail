// <input type="datetime-local"> carries local wall-clock time with no zone; the
// API takes RFC3339. Convert at the boundary.
export function isoToLocalInputValue(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function localInputValueToISO(value: string): string | null {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) {
    return null
  }
  return d.toISOString()
}
