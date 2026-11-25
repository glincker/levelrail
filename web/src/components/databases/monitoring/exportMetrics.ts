import type { ChartDef, SeriesMap } from './chartRegistry'
import { buildRows, isCollected } from './chartRegistry'

export interface ExportColumn {
  id: string
  label: string
}

export interface ExportTable {
  columns: ExportColumn[]
  rows: Array<Record<string, number | string>>
}

// Joins every visible collected series on timestamp into one table.
export function buildExportTable(
  defs: readonly ChartDef[],
  data: SeriesMap,
  labelOf: (def: ChartDef, seriesIndex: number) => string,
): ExportTable {
  const columns: ExportColumn[] = []
  const byTime = new Map<number, Record<string, number | string>>()
  for (const def of defs.filter(isCollected)) {
    const rows = buildRows(def, data)
    def.series.forEach((s, i) => {
      const id = s.metric
      columns.push({ id, label: labelOf(def, i) })
      for (const r of rows) {
        const v = r[`s${i}`]
        if (v === undefined) continue
        const row = byTime.get(r.t) ?? {
          timestamp: new Date(r.t).toISOString(),
        }
        row[id] = v
        byTime.set(r.t, row)
      }
    })
  }
  const rows = [...byTime.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([, row]) => row)
  return { columns, rows }
}

function csvCell(value: string | number | undefined): string {
  if (value === undefined) return ''
  const s = String(value)
  return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

export function toCsv(table: ExportTable): string {
  const head = ['timestamp', ...table.columns.map((c) => c.label)]
  const lines = [head.map(csvCell).join(',')]
  for (const row of table.rows) {
    lines.push(
      [row.timestamp, ...table.columns.map((c) => row[c.id])]
        .map(csvCell)
        .join(','),
    )
  }
  return lines.join('\n') + '\n'
}

export function toJson(table: ExportTable): string {
  const out = table.rows.map((row) => {
    const o: Record<string, number | string | null> = {
      timestamp: String(row.timestamp),
    }
    for (const c of table.columns) o[c.label] = row[c.id] ?? null
    return o
  })
  return JSON.stringify(out, null, 2)
}

export function downloadText(
  filename: string,
  mime: string,
  text: string,
): void {
  const url = URL.createObjectURL(new Blob([text], { type: mime }))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
