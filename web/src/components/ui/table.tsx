'use client'

import * as React from 'react'
import { useState } from 'react'

import { cn } from '@/lib/utils'

export type TableDensity = 'compact' | 'balanced' | 'spacious'

/** Right aligned, tabular figures for numeric cells. */
export const TABLE_NUMERIC = 'text-right tabular-nums'

function Table({
  className,
  containerClassName,
  density = 'balanced',
  ...props
}: React.ComponentProps<'table'> & {
  containerClassName?: string
  density?: TableDensity
}) {
  const [scrolled, setScrolled] = useState(false)
  return (
    <div
      data-slot="table-container"
      data-scrolled={scrolled || undefined}
      onScroll={(e) => setScrolled(e.currentTarget.scrollTop > 0)}
      className={cn(
        'relative w-full overflow-x-auto overscroll-x-contain [&_thead_th]:sticky [&_thead_th]:top-0 [&_thead_th]:z-10 [&_thead_th]:bg-background [&_thead_th]:transition-shadow motion-reduce:[&_thead_th]:transition-none data-scrolled:[&_thead_th]:bg-glinr-material-thick data-scrolled:[&_thead_th]:shadow-[0_1px_0_var(--border)]',
        containerClassName,
      )}
    >
      <table
        data-density={density}
        data-slot="table"
        className={cn(
          'group/table w-full caption-bottom text-sm',
          'max-md:[&_tr>:first-child]:sticky max-md:[&_tr>:first-child]:left-0 max-md:[&_tr>:first-child]:z-10 max-md:[&_tr>:first-child]:bg-background',
          className,
        )}
        {...props}
      />
    </div>
  )
}

function TableHeader({ className, ...props }: React.ComponentProps<'thead'>) {
  return (
    <thead
      data-slot="table-header"
      className={cn('[&_tr]:border-b [&_tr]:border-border', className)}
      {...props}
    />
  )
}

function TableBody({ className, ...props }: React.ComponentProps<'tbody'>) {
  return (
    <tbody
      data-slot="table-body"
      className={cn('[&_tr:last-child]:border-0', className)}
      {...props}
    />
  )
}

function TableFooter({ className, ...props }: React.ComponentProps<'tfoot'>) {
  return (
    <tfoot
      data-slot="table-footer"
      className={cn(
        'border-t bg-muted/50 font-medium [&>tr]:last:border-b-0',
        className,
      )}
      {...props}
    />
  )
}

function TableRow({ className, ...props }: React.ComponentProps<'tr'>) {
  return (
    <tr
      data-slot="table-row"
      className={cn(
        'border-b border-border/60 transition-colors duration-100 hover:bg-glinr-hover has-aria-expanded:bg-glinr-hover data-[state=selected]:bg-glinr-accent-soft data-[state=selected]:[&>td:first-child]:shadow-[inset_2px_0_0_var(--state-active-bar)] aria-selected:bg-glinr-accent-soft aria-selected:[&>td:first-child]:shadow-[inset_2px_0_0_var(--state-active-bar)]',
        className,
      )}
      {...props}
    />
  )
}

function TableHead({ className, ...props }: React.ComponentProps<'th'>) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        'h-9 px-3 py-2 text-left align-middle text-xs font-medium whitespace-nowrap text-muted-foreground [&:has([role=checkbox])]:pr-0',
        className,
      )}
      {...props}
    />
  )
}

function TableCell({ className, ...props }: React.ComponentProps<'td'>) {
  return (
    <td
      data-slot="table-cell"
      className={cn(
        'h-10 px-3 py-2 align-middle whitespace-nowrap group-data-[density=compact]/table:h-8 group-data-[density=compact]/table:py-1 group-data-[density=spacious]/table:h-12 [&:has([role=checkbox])]:pr-0',
        className,
      )}
      {...props}
    />
  )
}

function TableCaption({
  className,
  ...props
}: React.ComponentProps<'caption'>) {
  return (
    <caption
      data-slot="table-caption"
      className={cn('mt-4 text-sm text-muted-foreground', className)}
      {...props}
    />
  )
}

export {
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableHead,
  TableRow,
  TableCell,
  TableCaption,
}
