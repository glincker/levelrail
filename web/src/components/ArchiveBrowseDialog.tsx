import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useArchiveRows } from '../queries/archives'
import type { ArchiveBundle } from '../queries/archives'

const PAGE = 25

function cell(v: unknown): string {
  if (v === null || v === undefined) return ''
  return typeof v === 'string' ? v : JSON.stringify(v)
}

export function ArchiveBrowseDialog({
  bundle,
  onClose,
}: {
  bundle: ArchiveBundle | null
  onClose: () => void
}) {
  const { t } = useTranslation('archives')
  const tables = bundle ? Object.keys(bundle.rows).sort() : []
  const [table, setTable] = useState('')
  const [offset, setOffset] = useState(0)
  const active =
    table !== '' && tables.includes(table) ? table : (tables[0] ?? '')
  const rows = useArchiveRows(bundle?.id ?? '', bundle ? active : '', offset)
  const total = rows.data?.total ?? 0
  const columns = rows.data?.rows[0] ? Object.keys(rows.data.rows[0]) : []

  return (
    <Dialog
      open={bundle !== null}
      onOpenChange={(open) => {
        if (!open) {
          setTable('')
          setOffset(0)
          onClose()
        }
      }}
    >
      <DialogContent className="sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle className="font-mono text-sm">
            {t('browseDialog.title', { id: bundle?.id ?? '' })}
          </DialogTitle>
        </DialogHeader>
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs text-muted-foreground">
            {t('browseDialog.table')}
          </span>
          {tables.map((name) => (
            <Button
              key={name}
              size="sm"
              variant={name === active ? 'default' : 'outline'}
              onClick={() => {
                setTable(name)
                setOffset(0)
              }}
            >
              {name}
            </Button>
          ))}
        </div>
        <div className="max-h-[50vh] overflow-auto rounded-lg border border-border">
          <table className="w-full text-left text-xs">
            <thead className="sticky top-0 bg-card text-muted-foreground">
              <tr>
                {columns.map((c) => (
                  <th key={c} className="px-2 py-1.5 font-medium">
                    {c}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {(rows.data?.rows ?? []).map((r, i) => (
                <tr key={`${offset}-${i}`}>
                  {columns.map((c) => (
                    <td
                      key={c}
                      className="max-w-64 truncate px-2 py-1.5 font-mono"
                      title={cell(r[c])}
                    >
                      {cell(r[c])}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <DialogFooter className="items-center sm:justify-between">
          <span className="text-xs text-muted-foreground">
            {t('browseDialog.page', {
              from: total === 0 ? 0 : offset + 1,
              to: Math.min(offset + PAGE, total),
              total,
            })}
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={offset === 0}
              onClick={() => {
                setOffset(Math.max(0, offset - PAGE))
              }}
            >
              {t('browseDialog.prev')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={offset + PAGE >= total}
              onClick={() => {
                setOffset(offset + PAGE)
              }}
            >
              {t('browseDialog.next')}
            </Button>
            <Button variant="outline" size="sm" onClick={onClose}>
              {t('browseDialog.close')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
