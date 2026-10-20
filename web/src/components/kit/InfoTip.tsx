import { useId, useState, type ReactNode } from 'react'
import { InfoIcon } from '@phosphor-icons/react/dist/ssr'
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

export function InfoTip({
  children,
  label = 'More info',
  side = 'top',
  helpPath,
  helpLabel,
}: InfoTipProps) {
  const [open, setOpen] = useState(false)
  const id = useId()
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        aria-label={label}
        aria-describedby={open ? id : undefined}
        openOnHover
        delay={200}
        className="inline-flex size-5 shrink-0 items-center justify-center rounded-full text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
      >
        <InfoIcon className="size-3.5" aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent
        id={id}
        side={side}
        className="w-64 rounded-xl p-3 text-xs leading-relaxed shadow-floating"
      >
        {children}
        {helpPath ? (
          <HelpLink
            path={helpPath}
            label={helpLabel ?? 'Learn more'}
            variant="inline"
            className="mt-2"
          />
        ) : null}
      </PopoverContent>
    </Popover>
  )
}
