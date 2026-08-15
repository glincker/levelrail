'use client'

import * as React from 'react'
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { XIcon } from '@phosphor-icons/react/dist/ssr'

function Dialog({ ...props }: DialogPrimitive.Root.Props) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />
}

function DialogTrigger({ ...props }: DialogPrimitive.Trigger.Props) {
  return <DialogPrimitive.Trigger data-slot="dialog-trigger" {...props} />
}

function DialogPortal({ ...props }: DialogPrimitive.Portal.Props) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />
}

function DialogClose({ ...props }: DialogPrimitive.Close.Props) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />
}

function DialogOverlay({
  className,
  ...props
}: DialogPrimitive.Backdrop.Props) {
  return (
    <DialogPrimitive.Backdrop
      data-slot="dialog-overlay"
      className={cn(
        'fixed inset-0 isolate z-50 bg-black/10 duration-100 supports-backdrop-filter:backdrop-blur-xs data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0',
        className,
      )}
      {...props}
    />
  )
}

// `size` follows the same cva shape as buttonVariants (button.tsx):
// a base class string every popup gets regardless of variant, plus a
// `variants.size` map for the parts that actually differ.
//
// The base intentionally keeps `p-4` fixed across both variants rather
// than letting `fullscreen` claim more padding for itself. DialogFooter
// below bleeds to the popup's edges with `-mx-4 -mb-4`, a value hardcoded
// against this exact padding; if `fullscreen` used a different padding
// scale, that bleed math would drift out from under it and the footer
// would either leave a gap or overshoot. A consumer that wants a more
// spacious fullscreen feel (CreateResourceWizard does) layers its own
// padding on an inner wrapper instead, which keeps this primitive honest
// about what it controls: position, size, corner treatment, and scroll,
// not the padding rhythm of whatever content gets put inside it.
//
// `fullscreen` keeps the same `rounded-xl` as `default` for the same
// reason: DialogFooter's own bottom corners are hardcoded to
// `rounded-b-xl`, not parameterized by size. Going edge-to-edge with
// square corners would leave that hardcoded radius rendering as a small
// notch of visible backdrop in the popup's bottom corners. Keeping a
// small inset (`inset-4`/`inset-6`) instead of `inset-0` gives
// "fills the viewport" the deliberately-smaller-radius-with-an-inset
// treatment rather than a true edge-to-edge one, and sidesteps the
// mismatch entirely by never changing the radius at all.
const dialogContentVariants = cva(
  'fixed z-50 grid gap-4 rounded-xl bg-popover p-4 text-sm text-popover-foreground ring-1 ring-foreground/10 duration-100 outline-none data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95',
  {
    variants: {
      size: {
        // Exactly the classes this component hardcoded before the size
        // variant existed. Any dialog that doesn't pass `size` renders
        // byte-for-byte what it always has.
        default:
          'top-1/2 left-1/2 w-full max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 sm:max-w-sm',
        // No centering transform: `inset-*` positions and sizes the
        // popup directly against the viewport instead of a centered
        // fixed-width box, which is also what makes this fill available
        // space rather than just growing a fixed max-width. `overflow-y-auto`
        // is the popup's own scroll handling for a form taller than the
        // viewport; header, body, and footer aren't split into separate
        // scroll regions (that would mean reaching into whatever content
        // gets rendered inside, which this primitive doesn't and shouldn't
        // know about), so the whole popup scrolls together while the
        // close button stays pinned to the popup's frame because it's
        // positioned absolute against that same fixed-position ancestor.
        fullscreen: 'inset-4 w-auto max-w-none overflow-y-auto sm:inset-6',
      },
    },
    defaultVariants: {
      size: 'default',
    },
  },
)

function DialogContent({
  className,
  children,
  showCloseButton = true,
  size,
  ...props
}: DialogPrimitive.Popup.Props &
  VariantProps<typeof dialogContentVariants> & {
    showCloseButton?: boolean
  }) {
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Popup
        data-slot="dialog-content"
        className={cn(dialogContentVariants({ size, className }))}
        {...props}
      >
        {children}
        {showCloseButton && (
          <DialogPrimitive.Close
            data-slot="dialog-close"
            render={
              <Button
                variant="ghost"
                className="absolute top-2 right-2"
                size="icon-sm"
              />
            }
          >
            <XIcon />
            <span className="sr-only">Close</span>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Popup>
    </DialogPortal>
  )
}

function DialogHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="dialog-header"
      className={cn('flex flex-col gap-2', className)}
      {...props}
    />
  )
}

function DialogFooter({
  className,
  showCloseButton = false,
  children,
  ...props
}: React.ComponentProps<'div'> & {
  showCloseButton?: boolean
}) {
  return (
    <div
      data-slot="dialog-footer"
      className={cn(
        '-mx-4 -mb-4 flex flex-col-reverse gap-2 rounded-b-xl border-t bg-muted/50 p-4 sm:flex-row sm:justify-end',
        className,
      )}
      {...props}
    >
      {children}
      {showCloseButton && (
        <DialogPrimitive.Close render={<Button variant="outline" />}>
          Close
        </DialogPrimitive.Close>
      )}
    </div>
  )
}

function DialogTitle({ className, ...props }: DialogPrimitive.Title.Props) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      className={cn('text-base leading-none font-medium', className)}
      {...props}
    />
  )
}

function DialogDescription({
  className,
  ...props
}: DialogPrimitive.Description.Props) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      className={cn(
        'text-sm text-muted-foreground *:[a]:underline *:[a]:underline-offset-3 *:[a]:hover:text-foreground',
        className,
      )}
      {...props}
    />
  )
}

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
}
