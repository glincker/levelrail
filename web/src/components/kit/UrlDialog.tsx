import type { ReactNode, RefObject } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import { DEFAULT_DIALOG_PARAM } from '../../lib/urlDialog'
import { useUrlDialog } from './useUrlDialog'

export interface UrlDialogProps {
  /** Search param that carries the dialog value, default `connect`. */
  param?: string
  /** The value of `param` that opens this dialog. */
  value: string
  /** Extra params owned by the dialog, removed again on close. */
  ownedParams?: readonly string[]
  title: ReactNode
  description?: ReactNode
  /** Element focus returns to on close, usually the opening button. */
  returnFocusRef?: RefObject<HTMLElement | null>
  className?: string
  children: ReactNode
}

export function UrlDialog({
  param = DEFAULT_DIALOG_PARAM,
  value,
  ownedParams = [],
  title,
  description,
  returnFocusRef,
  className,
  children,
}: UrlDialogProps) {
  const { isOpen, close } = useUrlDialog(value, param, ownedParams)
  return (
    <Dialog
      open={isOpen}
      onOpenChange={(next) => {
        if (!next) close()
      }}
    >
      <DialogContent
        finalFocus={returnFocusRef}
        className={cn(
          'sm:max-w-xl motion-reduce:duration-0 motion-reduce:data-open:animate-none motion-reduce:data-closed:animate-none',
          className,
        )}
      >
        <DialogHeader className="pr-8">
          <DialogTitle>{title}</DialogTitle>
          {description ? (
            <DialogDescription>{description}</DialogDescription>
          ) : null}
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  )
}
