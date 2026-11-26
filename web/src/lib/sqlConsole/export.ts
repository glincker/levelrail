// Client-side CSV and JSON export of a result page. Cells arrive as
// strings (or null) from the API; numbers and bigints are accepted too.

export type ExportCell = string | number | bigint | boolean | null

const PLAIN_NUMBER = /^[+-]?\d+(\.\d+)?([eE][+-]?\d+)?$/
const FORMULA_START = /^[=+\-@\t\r]/

function cellText(cell: ExportCell): string {
  return cell === null ? '' : String(cell)
}

// Spreadsheet formula injection guard. A plain number such as -5 is left
// alone so negative values survive the export.
function neutralize(text: string): string {
  if (FORMULA_START.test(text) && !PLAIN_NUMBER.test(text)) {
    return `'${text}`
  }
  return text
}

function csvField(cell: ExportCell): string {
  const text = neutralize(cellText(cell))
  if (/[",\r\n]/.test(text) || text !== text.trim()) {
    return `"${text.replace(/"/g, '""')}"`
  }
  return text
}

export function toCsv(columns: string[], rows: ExportCell[][]): string {
  const lines = [columns.map(csvField).join(',')]
  for (const row of rows) {
    lines.push(columns.map((_, i) => csvField(row[i] ?? null)).join(','))
  }
  return lines.join('\r\n') + '\r\n'
}

// Duplicate column names get a numeric suffix so no value is dropped.
function uniqueKeys(columns: string[]): string[] {
  const seen = new Map<string, number>()
  return columns.map((c) => {
    const n = (seen.get(c) ?? 0) + 1
    seen.set(c, n)
    return n === 1 ? c : `${c}_${String(n)}`
  })
}

export function toJson(columns: string[], rows: ExportCell[][]): string {
  const keys = uniqueKeys(columns)
  const objects = rows.map((row) => {
    const o: Record<string, string | number | boolean | null> = {}
    keys.forEach((k, i) => {
      const v = row[i] ?? null
      o[k] = typeof v === 'bigint' ? v.toString() : v
    })
    return o
  })
  return JSON.stringify(objects, null, 2)
}

export function downloadText(filename: string, mime: string, text: string) {
  const url = URL.createObjectURL(
    new Blob([text], { type: `${mime};charset=utf-8` }),
  )
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
