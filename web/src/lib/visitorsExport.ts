// Filename and save helpers for the visitors CSV export.

const FILENAME_RE = /filename\*?=(?:UTF-8'')?"?([^";]+)"?/i

export function visitorsExportFilename(
  disposition: string | null,
  app: string,
  window: string,
): string {
  const m = disposition ? FILENAME_RE.exec(disposition) : null
  const raw = m?.[1]?.trim() ?? ''
  // Strip path separators so a header can never pick a directory.
  const name = raw.replace(/[\\/]/g, '')
  return name !== '' ? name : `visitors-${app}-${window}.csv`
}

export function saveBlobAs(blob: Blob, name: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}
