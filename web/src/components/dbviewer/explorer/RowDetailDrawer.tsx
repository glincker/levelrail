import { useTranslation } from 'react-i18next'
import {
  CaretLeftIcon,
  CaretRightIcon,
  CopyIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { prettyJson } from '../../../lib/explorerData'
import type { DbCell } from '../../../types/databaseViewer'
import { ColumnTypeIcon } from './ColumnTypeIcon'
import type { GridColumn } from './DataGrid'
import { useCopy } from './useCopy'

function FieldValue({ value }: { value: DbCell }) {
  const { t } = useTranslation('databases')
  if (value === null) {
    return (
      <span className="rounded bg-muted px-1.5 py-0.5 text-xs font-medium italic text-muted-foreground">
        {t('viewer.grid.null')}
      </span>
    )
  }
  if (value === '') {
    return (
      <span className="font-mono text-sm text-muted-foreground">
        {'""'} <span className="text-xs">{t('viewer.grid.emptyString')}</span>
      </span>
    )
  }
  const pretty = prettyJson(value)
  return (
    <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted p-2 font-mono text-xs">
      {pretty ?? value}
    </pre>
  )
}

export function RowDetailDrawer({
  open,
  onOpenChange,
  title,
  columns,
  rows,
  rowIndex,
  onRowChange,
  cellCapBytes,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  columns: GridColumn[]
  rows: DbCell[][]
  rowIndex: number
  onRowChange: (index: number) => void
  cellCapBytes?: number
}) {
  const { t } = useTranslation('databases')
  const copy = useCopy()
  const row = rows[rowIndex]

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>
            {t('viewer.grid.rowOf', {
              row: rowIndex + 1,
              total: rows.length,
            })}
          </SheetDescription>
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={rowIndex <= 0}
              onClick={() => {
                onRowChange(rowIndex - 1)
              }}
            >
              <CaretLeftIcon className="size-3.5" aria-hidden="true" />
              {t('viewer.grid.previousRow')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={rowIndex >= rows.length - 1}
              onClick={() => {
                onRowChange(rowIndex + 1)
              }}
            >
              {t('viewer.grid.nextRow')}
              <CaretRightIcon className="size-3.5" aria-hidden="true" />
            </Button>
          </div>
        </SheetHeader>
        <dl className="space-y-3 px-4 pb-6">
          {columns.map((col, i) => {
            const value = row?.[i] ?? null
            return (
              <div key={col.name} className="space-y-1">
                <dt className="flex items-center gap-1.5 text-xs font-medium">
                  <ColumnTypeIcon kind={col.kind} type={col.type} />
                  <span>{col.name}</span>
                  {col.type ? (
                    <span className="font-normal text-muted-foreground">
                      {col.type}
                    </span>
                  ) : null}
                  {value !== null ? (
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      className="ml-auto size-6"
                      aria-label={t('viewer.grid.copyValue', {
                        column: col.name,
                      })}
                      onClick={() => {
                        copy(value)
                      }}
                    >
                      <CopyIcon className="size-3.5" aria-hidden="true" />
                    </Button>
                  ) : null}
                </dt>
                <dd>
                  <FieldValue value={value} />
                  {cellCapBytes &&
                  value !== null &&
                  new Blob([value]).size >= cellCapBytes ? (
                    <p className="mt-1 text-xs text-tone-warning">
                      {t('viewer.grid.cellCapped')}
                    </p>
                  ) : null}
                </dd>
              </div>
            )
          })}
        </dl>
      </SheetContent>
    </Sheet>
  )
}
