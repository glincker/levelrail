import { useState, type ReactNode } from 'react'
import { ChevronRightGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { DOT, type WireTone } from './tlsDot'

const LINE: Record<WireTone, string> = {
  ok: 'bg-tone-success-solid',
  warn: 'bg-tone-warning-solid',
  bad: 'bg-tone-danger-solid',
  off: 'bg-muted-foreground/35',
}
const INK: Record<WireTone, string> = {
  ok: 'text-tone-success',
  warn: 'text-tone-warning',
  bad: 'text-tone-danger',
  off: 'text-muted-foreground/50',
}

/** Tracks the left-to-right flow: lines only exist where the three columns sit side by side. */
export const WIRING_GRID =
  'grid gap-3 @xl:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)_minmax(0,1fr)] @xl:items-center @xl:gap-x-8'

/** Column gap is 1.5 rem so spine segments overshoot by that much to join their neighbours. */
export const WIRING_COLUMN = 'flex min-w-0 flex-col gap-1.5'

const ARROW =
  'absolute size-0 border-y-4 border-y-transparent border-l-[6px] border-l-current'

/** Left column connector: a dot on the item, a stub to the spine, and the spine joined to its neighbours. */
function LeftWire({ tone }: Readonly<{ tone: WireTone }>) {
  return (
    <span aria-hidden="true" className="hidden @xl:block">
      <span className={`absolute top-3.5 -right-4 h-px w-4 ${LINE[tone]}`} />
      <span
        className={`absolute top-3.5 -right-0.5 size-1 -translate-y-1/2 rounded-full ${DOT[tone]}`}
      />
      <span className="absolute -right-4 -top-1.5 -bottom-1.5 w-px bg-muted-foreground/35 group-first/w:top-3.5 group-last/w:bottom-auto group-last/w:h-5 group-only/w:hidden" />
    </span>
  )
}

/** Right column connector: spine, stub and an arrowhead landing on the row. */
function RightWire({ tone }: Readonly<{ tone: WireTone }>) {
  return (
    <span aria-hidden="true" className="hidden @xl:block">
      <span className={`absolute top-1/2 -left-4 h-px w-4 ${LINE[tone]}`} />
      <span
        className={`${ARROW} top-1/2 -left-1 -translate-y-1/2 ${INK[tone]}`}
      />
      <span className="absolute -left-4 -top-1.5 -bottom-1.5 w-px bg-muted-foreground/35 group-first/w:top-1/2 group-last/w:bottom-1/2 group-only/w:hidden" />
    </span>
  )
}

/** Wires hang off the centre card: an arrow arriving on its left, a dot leaving on its right. */
export function CenterWires({ tone }: Readonly<{ tone: WireTone }>) {
  return (
    <span aria-hidden="true" className="hidden @xl:block">
      <span className={`absolute top-1/2 -left-4 h-px w-4 ${LINE[tone]}`} />
      <span
        className={`${ARROW} top-1/2 -left-px -translate-y-1/2 ${INK[tone]}`}
      />
      <span className={`absolute top-1/2 -right-4 h-px w-4 ${LINE[tone]}`} />
      <span
        className={`absolute top-1/2 -right-0.5 size-1 -translate-y-1/2 rounded-full ${DOT[tone]}`}
      />
    </span>
  )
}

function readOpen(key: string | undefined): boolean {
  if (!key) return true
  try {
    return window.localStorage.getItem(key) !== 'closed'
  } catch {
    return true
  }
}

function writeOpen(key: string | undefined, open: boolean) {
  if (!key) return
  try {
    window.localStorage.setItem(key, open ? 'open' : 'closed')
  } catch {
    // Storage can be blocked; the group just forgets its state.
  }
}

/** Collapsible left-column group with a count badge; its open state is remembered under storageKey. */
export function Group({
  title,
  count,
  tone = 'off',
  storageKey,
  children,
}: Readonly<{
  title: string
  count: number
  tone?: WireTone
  storageKey?: string
  children: ReactNode
}>) {
  const [open, setOpen] = useState(() => readOpen(storageKey))
  return (
    <Collapsible
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        writeOpen(storageKey, next)
      }}
      className="group/w relative min-w-0"
    >
      <CollapsibleTrigger className="group/trg flex h-7 items-center gap-1.5 rounded-md text-xs font-medium focus-visible:ring-2 focus-visible:ring-ring">
        <ChevronRightGlassIcon
          aria-hidden="true"
          className="size-3.5 text-muted-foreground transition-transform duration-150 group-data-[panel-open]/trg:rotate-90 motion-reduce:transition-none"
        />
        {title}
        <span className="ml-auto min-w-5 rounded-full bg-muted px-1.5 text-center text-[0.6875rem] leading-5 text-muted-foreground tabular-nums">
          {count}
        </span>
      </CollapsibleTrigger>
      <CollapsiblePanel className="space-y-1 pt-0.5 pl-4">
        {children}
      </CollapsiblePanel>
      <LeftWire tone={tone} />
    </Collapsible>
  )
}

/** One compact row for an empty group: a small plus button and a short hint. */
export function EmptyRow({
  text,
  action,
  onAction,
  actionNode,
}: Readonly<{
  text: string
  action: string
  onAction?: () => void
  actionNode?: ReactNode
}>) {
  return (
    <div className="flex min-w-0 items-center gap-1.5">
      {actionNode ?? (
        <Button
          size="icon-xs"
          variant="outline"
          aria-label={action}
          title={action}
          onClick={onAction}
        >
          <PlusGlassIcon aria-hidden="true" />
        </Button>
      )}
      <p className="min-w-0 truncate text-xs text-muted-foreground">{text}</p>
    </div>
  )
}

/** Right column row: icon, label (the caller supplies the link), value and a state chip. */
export function WireItem({
  tone,
  icon,
  label,
  value,
  chip,
}: Readonly<{
  tone: WireTone
  icon: ReactNode
  label: ReactNode
  value?: ReactNode
  chip: string
}>) {
  return (
    <div className="group/w relative flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 rounded-md border border-border px-2 py-1.5 text-xs">
      <span
        aria-hidden="true"
        className={`flex size-5 shrink-0 items-center justify-center rounded bg-muted [&_svg]:size-3.5 ${tone === 'off' ? 'text-muted-foreground' : 'text-foreground'}`}
      >
        {icon}
      </span>
      <span className="min-w-24 flex-1">
        <span className="block truncate font-medium">{label}</span>
        {value ? (
          <span className="block truncate text-muted-foreground">{value}</span>
        ) : null}
      </span>
      <ToneChip tone={tone}>{chip}</ToneChip>
      <RightWire tone={tone} />
    </div>
  )
}

const CHIP_VARIANT = {
  ok: 'success',
  warn: 'warning',
  bad: 'destructive',
  off: 'muted',
} as const

export function ToneChip({
  tone,
  children,
}: Readonly<{ tone: WireTone; children: ReactNode }>) {
  return (
    <Badge variant={CHIP_VARIANT[tone]} className="shrink-0">
      {children}
    </Badge>
  )
}

/** Status dot that pulses while live; the ring stops for reduced motion. */
export function LiveDot({
  tone,
  pulse,
}: Readonly<{ tone: WireTone; pulse: boolean }>) {
  return (
    <span aria-hidden="true" className="relative flex size-2">
      {pulse ? (
        <span
          className={`absolute inset-0 rounded-full opacity-60 motion-safe:animate-ping ${DOT[tone]}`}
        />
      ) : null}
      <span className={`relative size-2 rounded-full ${DOT[tone]}`} />
    </span>
  )
}

/** Toggle-like setting chip with an icon; on is accent, off is muted. */
export function SettingChip({
  icon,
  label,
  on,
  onLabel,
  offLabel,
  onClick,
}: Readonly<{
  icon: ReactNode
  label: string
  on: boolean
  onLabel: string
  offLabel: string
  onClick: () => void
}>) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`inline-flex h-6 items-center gap-1 rounded-full border px-2 text-xs transition-colors focus-visible:ring-2 focus-visible:ring-ring motion-reduce:transition-none [&_svg]:size-3 ${
        on
          ? 'border-tone-accent-border bg-tone-accent-soft text-tone-accent'
          : 'border-border bg-muted text-muted-foreground hover:text-foreground'
      }`}
    >
      {icon}
      {label}
      <span className="sr-only">: </span>
      <span className="font-medium">{on ? onLabel : offLabel}</span>
    </button>
  )
}
