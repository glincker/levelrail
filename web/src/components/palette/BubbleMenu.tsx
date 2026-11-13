import * as React from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftGlassIcon,
  ArrowRightGlassIcon,
  ExternalLinkGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The bubble button, so focus returns to it when the menu closes. */
  returnFocus: () => HTMLElement | null
  pinned: boolean
  href?: string
  canMoveLeft: boolean
  canMoveRight: boolean
  onUnpin: () => void
  onMoveLeft: () => void
  onMoveRight: () => void
  onOpenNewTab: () => void
}

// Right-click, long-press and Shift+F10 menu for one bubble, anchored to an invisible trigger.
export function BubbleMenu(p: Props) {
  const { t } = useTranslation('palette')
  return (
    <DropdownMenu open={p.open} onOpenChange={p.onOpenChange}>
      <DropdownMenuTrigger
        render={
          <span
            tabIndex={-1}
            className="pointer-events-none absolute inset-0 rounded-full"
          />
        }
      />
      <DropdownMenuContent
        align="center"
        side="bottom"
        finalFocus={p.returnFocus}
        onKeyDown={(e) => {
          // Esc closes only the menu; the palette stays open behind it.
          if (e.key === 'Escape') e.stopPropagation()
        }}
        aria-label={t('bubbles.menu')}
      >
        {p.pinned ? (
          <DropdownMenuItem onClick={p.onUnpin}>
            <XGlassIcon size={16} aria-hidden="true" />
            {t('bubbles.unpin')}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem disabled={!p.canMoveLeft} onClick={p.onMoveLeft}>
          <ArrowLeftGlassIcon size={16} aria-hidden="true" />
          {t('bubbles.moveLeft')}
        </DropdownMenuItem>
        <DropdownMenuItem disabled={!p.canMoveRight} onClick={p.onMoveRight}>
          <ArrowRightGlassIcon size={16} aria-hidden="true" />
          {t('bubbles.moveRight')}
        </DropdownMenuItem>
        {p.href ? (
          <DropdownMenuItem onClick={p.onOpenNewTab}>
            <ExternalLinkGlassIcon size={16} aria-hidden="true" />
            {t('bubbles.openNewTab')}
          </DropdownMenuItem>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
