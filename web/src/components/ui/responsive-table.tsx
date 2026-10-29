import {
  useEffect,
  useRef,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
} from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { EllipsisGlassIcon } from '@thesvg/ui-react'
import { ActionMenu, type ActionMenuItem } from '@/components/kit'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useMediaQuery } from '@/hooks/useMediaQuery'
import { cn } from '@/lib/utils'
import {
  DEFAULT_VIRTUALIZE_OVER,
  cardFields,
  paddingFor,
  priorityClass,
  shouldVirtualize,
  splitColumns,
  type ColumnPriority,
} from '@/lib/responsiveTable'

/** Put on a link inside a row so the whole row is its click target. */
export const ROW_LINK_OVERLAY = 'after:absolute after:inset-0'

export interface ResponsiveColumn<T> {
  id: string
  header: ReactNode
  cell: (row: T) => ReactNode
  /** 1 always shows, 2 hides below a 42rem container, 3 below a 56rem one. */
  priority?: ColumnPriority
  /** Card title below md. Defaults to the first column. */
  primary?: boolean
  /** Tailwind width class for fixed layout, e.g. `w-32`. */
  width?: string
  align?: 'start' | 'end'
  hideInCard?: boolean
  /** Card label when `header` is not a plain string. */
  cardLabel?: string
}

export interface ResponsiveRowProps {
  className?: string
  'aria-current'?: 'true'
  'data-kind'?: string
}

export interface ResponsiveTableProps<T> {
  rows: readonly T[]
  columns: readonly ResponsiveColumn<T>[]
  getRowId: (row: T) => string
  ariaLabel: string
  /** Fixed leading cell (logo tile, checkbox) shown beside the title in cards. */
  leading?: {
    header?: ReactNode
    cell: (row: T) => ReactNode
    width?: string
  }
  /** Row actions collapse into one kebab menu. */
  actions?: (row: T) => ActionMenuItem[]
  actionsLabel?: (row: T) => string
  /** Escape hatch for rows whose actions are not a plain menu. */
  renderActions?: (row: T) => ReactNode
  onRowClick?: (row: T) => void
  getRowProps?: (row: T) => ResponsiveRowProps
  focusRowId?: string
  virtualizeOver?: number
  /** Tailwind max-height class for the scroll area when virtualized. */
  scrollHeightClass?: string
  /** Opt in to sideways scrolling instead of fitting the container. */
  horizontalScroll?: boolean
  estimateRowHeight?: number
  /** Lets the page read or drive the scroll position. */
  scrollRef?: RefObject<HTMLDivElement | null>
  className?: string
}

const INTERACTIVE = 'a,button,input,select,textarea,[role=menuitem]'

function RowActions<T>({
  row,
  actions,
  actionsLabel,
  renderActions,
}: Pick<
  ResponsiveTableProps<T>,
  'actions' | 'actionsLabel' | 'renderActions'
> & { row: T }) {
  if (renderActions) {
    return <span className="relative z-10">{renderActions(row)}</span>
  }
  const items = actions?.(row) ?? []
  if (items.length === 0) return null
  return (
    <span className="relative z-10">
      <ActionMenu
        trigger={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={actionsLabel?.(row) ?? 'Row actions'}
          >
            <EllipsisGlassIcon size="1.2em" aria-hidden="true" />
          </Button>
        }
        items={items}
      />
    </span>
  )
}

function useRowActivation<T>(onRowClick: ((row: T) => void) | undefined) {
  return (row: T) =>
    onRowClick
      ? {
          tabIndex: 0,
          onClick: (e: React.MouseEvent<HTMLElement>) => {
            if ((e.target as HTMLElement).closest(INTERACTIVE)) return
            onRowClick(row)
          },
          onKeyDown: (e: KeyboardEvent<HTMLElement>) => {
            if (e.target !== e.currentTarget) return
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              onRowClick(row)
            }
          },
        }
      : {}
}

export function ResponsiveTable<T>(props: ResponsiveTableProps<T>) {
  const {
    rows,
    columns,
    getRowId,
    ariaLabel,
    leading,
    onRowClick,
    getRowProps,
    focusRowId,
    virtualizeOver = DEFAULT_VIRTUALIZE_OVER,
    scrollHeightClass = 'max-h-[70vh]',
    horizontalScroll = false,
    estimateRowHeight = 56,
    className,
  } = props
  const tableMode = useMediaQuery('(min-width: 768px)')
  const ownRef = useRef<HTMLDivElement>(null)
  const scrollRef = props.scrollRef ?? ownRef
  const virtual = shouldVirtualize(rows.length, virtualizeOver)
  const virtualizer = useVirtualizer({
    count: virtual ? rows.length : 0,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => estimateRowHeight,
    overscan: 8,
    getItemKey: (i) => {
      const row = rows[i]
      return row === undefined ? i : getRowId(row)
    },
  })
  const activation = useRowActivation(onRowClick)
  const hasActions = Boolean(props.actions ?? props.renderActions)

  useEffect(() => {
    if (!focusRowId) return
    const idx = rows.findIndex((r) => getRowId(r) === focusRowId)
    if (idx < 0) return
    if (virtual) {
      virtualizer.scrollToIndex(idx, { align: 'auto' })
      return
    }
    scrollRef.current
      ?.querySelector(`[data-row-id="${CSS.escape(focusRowId)}"]`)
      ?.scrollIntoView?.({ block: 'nearest' })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusRowId])

  const slice = virtual
    ? virtualizer.getVirtualItems().flatMap((v) => {
        const row = rows[v.index]
        return row === undefined ? [] : [{ row, index: v.index }]
      })
    : rows.map((row, index) => ({ row, index }))
  const pad = virtual
    ? paddingFor(virtualizer.getVirtualItems(), virtualizer.getTotalSize())
    : { top: 0, bottom: 0 }
  const { primary } = splitColumns(columns)
  const fields = cardFields(columns)

  // Spacer heights are measured at runtime, so they cannot be Tailwind classes.
  const spacer = (h: number) => ({ height: h })

  const scrollClass = cn(
    '@container/rtable rounded-lg border border-border bg-card',
    virtual || horizontalScroll
      ? cn('overflow-auto', scrollHeightClass)
      : 'overflow-x-clip',
    className,
  )

  if (!tableMode) {
    return (
      <div ref={scrollRef} className={scrollClass}>
        <ul aria-label={ariaLabel} className="divide-y divide-border">
          {pad.top > 0 ? (
            <li aria-hidden="true" style={spacer(pad.top)} />
          ) : null}
          {slice.map(({ row, index }) => {
            const id = getRowId(row)
            const rp = getRowProps?.(row)
            return (
              <li
                key={id}
                data-row-id={id}
                data-index={index}
                ref={virtual ? virtualizer.measureElement : undefined}
                aria-current={rp?.['aria-current']}
                data-kind={rp?.['data-kind']}
                className={cn(
                  'relative space-y-2 p-3 transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset',
                  onRowClick && 'cursor-pointer',
                  rp?.className,
                )}
                {...activation(row)}
              >
                <div className="flex items-start gap-3">
                  {leading ? leading.cell(row) : null}
                  <div className="min-w-0 flex-1 text-sm font-medium">
                    {primary?.cell(row)}
                  </div>
                  {hasActions ? (
                    <RowActions
                      row={row}
                      actions={props.actions}
                      actionsLabel={props.actionsLabel}
                      renderActions={props.renderActions}
                    />
                  ) : null}
                </div>
                {fields.length > 0 ? (
                  <dl className="grid grid-cols-[minmax(5rem,auto)_minmax(0,1fr)] items-center gap-x-3 gap-y-1.5 text-sm">
                    {fields.map((c) => (
                      <div key={c.id} className="contents">
                        <dt className="text-xs text-muted-foreground">
                          {c.cardLabel ??
                            (typeof c.header === 'string' ? c.header : '')}
                        </dt>
                        <dd className="min-w-0 break-words">{c.cell(row)}</dd>
                      </div>
                    ))}
                  </dl>
                ) : null}
              </li>
            )
          })}
          {pad.bottom > 0 ? (
            <li aria-hidden="true" style={spacer(pad.bottom)} />
          ) : null}
        </ul>
      </div>
    )
  }

  const colSpan = columns.length + (leading ? 1 : 0) + (hasActions ? 1 : 0)
  return (
    <div ref={scrollRef} className={scrollClass}>
      <Table
        aria-label={ariaLabel}
        aria-rowcount={virtual ? rows.length + 1 : undefined}
        containerClassName="overflow-visible"
        className={cn(
          'bg-card',
          horizontalScroll ? 'min-w-max' : 'table-fixed',
        )}
      >
        <TableHeader className="sticky top-0 z-10 bg-card shadow-[inset_0_-1px_0_var(--glinr-border)]">
          <TableRow className="hover:bg-transparent">
            {leading ? (
              <TableHead className={cn('w-10 pl-4', leading.width)}>
                {leading.header ?? <span className="sr-only">Select</span>}
              </TableHead>
            ) : null}
            {columns.map((c) => (
              <TableHead
                key={c.id}
                className={cn(
                  'text-xs font-medium text-muted-foreground',
                  c.align === 'end' && 'text-right',
                  c.width,
                  priorityClass(c.priority),
                )}
              >
                {c.header}
              </TableHead>
            ))}
            {hasActions ? (
              <TableHead
                className={
                  props.renderActions
                    ? cn(
                        'whitespace-nowrap pr-4',
                        horizontalScroll ? 'w-px' : 'w-36',
                      )
                    : 'w-12'
                }
              >
                <span className="sr-only">Actions</span>
              </TableHead>
            ) : null}
          </TableRow>
        </TableHeader>
        <TableBody>
          {pad.top > 0 ? (
            <tr aria-hidden="true" style={spacer(pad.top)}>
              <td colSpan={colSpan} />
            </tr>
          ) : null}
          {slice.map(({ row, index }) => {
            const id = getRowId(row)
            const rp = getRowProps?.(row)
            return (
              <TableRow
                key={id}
                data-row-id={id}
                data-index={index}
                ref={virtual ? virtualizer.measureElement : undefined}
                aria-current={rp?.['aria-current']}
                data-kind={rp?.['data-kind']}
                className={cn(
                  'relative focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset',
                  onRowClick && 'cursor-pointer',
                  rp?.className,
                )}
                {...activation(row)}
              >
                {leading ? (
                  <TableCell className={cn('pl-4 pr-2', leading.width)}>
                    {leading.cell(row)}
                  </TableCell>
                ) : null}
                {columns.map((c) => (
                  <TableCell
                    key={c.id}
                    className={cn(
                      'py-3 break-words whitespace-normal',
                      c.align === 'end' && 'text-right',
                      priorityClass(c.priority),
                    )}
                  >
                    {c.cell(row)}
                  </TableCell>
                ))}
                {hasActions ? (
                  <TableCell
                    className={cn(
                      'text-right',
                      props.renderActions && 'whitespace-nowrap pr-4',
                    )}
                  >
                    <RowActions
                      row={row}
                      actions={props.actions}
                      actionsLabel={props.actionsLabel}
                      renderActions={props.renderActions}
                    />
                  </TableCell>
                ) : null}
              </TableRow>
            )
          })}
          {pad.bottom > 0 ? (
            <tr aria-hidden="true" style={spacer(pad.bottom)}>
              <td colSpan={colSpan} />
            </tr>
          ) : null}
        </TableBody>
      </Table>
    </div>
  )
}
