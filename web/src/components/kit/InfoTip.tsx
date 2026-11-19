import { useId, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { InfoGlassIcon } from '@thesvg/ui-react'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { HelpLink } from '@/components/HelpLink'

export interface InfoTipProps {
  children: ReactNode
  label?: string
  side?: 'top' | 'right' | 'bottom' | 'left'
  /** Docs path passed straight through to HelpLink; renders nothing if it resolves to neither a bundled page nor a configured docs URL. */
  helpPath?: string
  helpLabel?: string
}

// Opens on hover, focus or click; Escape closes it (the popover handles
// that). The popup floats, so opening never shifts the layout.
export function InfoTip({
  children,
  label,
  side = 'top',
  helpPath,
  helpLabel,
}: InfoTipProps) {
  const { t } = useTranslation('appPages', { useSuspense: false })
  const [open, setOpen] = useState(false)
  const id = useId()
  // Pointer focus is left to the click toggle so it does not open then close.
  const pointer = useRef(false)
  // Closing returns focus to the trigger; that must not reopen the tip.
  const closed = useRef(false)
  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        if (!next) closed.current = true
        setOpen(next)
      }}
    >
      <PopoverTrigger
        aria-label={label ?? t('tip.label', { defaultValue: 'More info' })}
        aria-describedby={open ? id : undefined}
        openOnHover
        delay={200}
        onPointerDown={() => {
          pointer.current = true
        }}
        onFocus={() => {
          if (!pointer.current && !closed.current) setOpen(true)
        }}
        onKeyDown={(e) => {
          // Already open from focus: Enter or Space must not toggle it shut.
          if (open && (e.key === 'Enter' || e.key === ' ')) e.preventDefault()
        }}
        onKeyUp={(e) => {
          if (open && e.key === ' ') e.preventDefault()
        }}
        onBlur={() => {
          pointer.current = false
          closed.current = false
        }}
        className="inline-flex size-5 shrink-0 items-center justify-center rounded-full text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
      >
        <InfoGlassIcon className="size-5 text-tone-info" aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent
        id={id}
        side={side}
        className="w-64 max-w-[calc(100vw-2rem)] rounded-xl p-3 text-xs leading-relaxed shadow-floating"
      >
        {children}
        {helpPath ? (
          <HelpLink
            path={helpPath}
            label={
              helpLabel ?? t('tip.learnMore', { defaultValue: 'Learn more' })
            }
            variant="inline"
            className="mt-2"
          />
        ) : null}
      </PopoverContent>
    </Popover>
  )
}
