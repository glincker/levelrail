'use client'

import '@/styles/material.css'
import * as React from 'react'
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'
import { nativeButtonFromRender } from '@/lib/nativeButton'
import { Button } from '@/components/ui/button'
import { XGlassIcon } from '@thesvg/ui-react'
function Dialog({ ...props }: DialogPrimitive.Root.Props) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />
}

function DialogTrigger({ ...props }: DialogPrimitive.Trigger.Props) {
  return (
    <DialogPrimitive.Trigger
      data-slot="dialog-trigger"
      {...props}
      nativeButton={nativeButtonFromRender(props.render, props.nativeButton)}
    />
  )
}

function DialogPortal({ ...props }: DialogPrimitive.Portal.Props) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />
}

function DialogClose({ ...props }: DialogPrimitive.Close.Props) {
  return (
    <DialogPrimitive.Close
      data-slot="dialog-close"
      {...props}
      nativeButton={nativeButtonFromRender(props.render, props.nativeButton)}
    />
  )
}

function DialogOverlay({
  className,
  ...props
}: DialogPrimitive.Backdrop.Props) {
  return (
    <DialogPrimitive.Backdrop
      data-slot="dialog-overlay"
      className={cn(
        'motion-scrim fixed inset-0 isolate z-50 bg-black/50 backdrop-blur-[2px] data-starting-style:opacity-0 data-ending-style:opacity-0',
        className,
      )}
      {...props}
    />
  )
}

// The base class pins the grid to one `minmax(0,1fr)` column. An implicit `auto`
// column grows to the widest unbreakable child (an image digest, a long path), which
// pushes lists, notes and the footer past the popup edge where `overflow-y-auto` clips them.
//
// `size` follows the same cva shape as buttonVariants (button.tsx):
// a base class string every popup gets regardless of variant, plus a
// `variants.size` map for the parts that actually differ.
//
// The base intentionally keeps the dialog pad token fixed across both variants rather
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
// `rounded-b-container`, not parameterized by size. Going edge-to-edge with
// square corners would leave that hardcoded radius rendering as a small
// notch of visible backdrop in the popup's bottom corners. Keeping a
// small inset (`inset-4`/`inset-6`) instead of `inset-0` gives
// "fills the viewport" the deliberately-smaller-radius-with-an-inset
// treatment rather than a true edge-to-edge one, and sidesteps the
// mismatch entirely by never changing the radius at all.
const dialogContentVariants = cva(
  'fixed z-50 grid grid-cols-[minmax(0,1fr)] gap-4 glass motion-overlay rounded-container p-(--glinr-dialog-pad) text-sm text-popover-foreground outline-none',
  {
    variants: {
      size: {
        // `max-h-[85vh]` plus `overflow-y-auto`: a centered popup with
        // content taller than the viewport (a long form, a growing
        // options grid) otherwise has no height ceiling at all and
        // overflows past both the top and bottom of the screen, pushing
        // its own header and close button out of view. Capping height
        // and scrolling internally is the same "the whole popup scrolls
        // together" contract `fullscreen` already documents below.
        default:
          'inset-x-0 bottom-0 max-h-[90dvh] w-full overflow-y-auto rounded-b-none pb-[max(var(--glinr-dialog-pad),env(safe-area-inset-bottom))] sm:inset-x-auto sm:bottom-auto sm:top-1/2 sm:left-1/2 sm:max-h-[85vh] sm:max-w-(--glinr-dialog-w-form) sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-b-container sm:pb-(--glinr-dialog-pad)',
        // Confirm and rich reuse the centered default and only change the width token.
        confirm:
          'inset-x-0 bottom-0 max-h-[90dvh] w-full overflow-y-auto rounded-b-none pb-[max(var(--glinr-dialog-pad),env(safe-area-inset-bottom))] sm:inset-x-auto sm:bottom-auto sm:top-1/2 sm:left-1/2 sm:max-h-[85vh] sm:max-w-(--glinr-dialog-w-confirm) sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-b-container sm:pb-(--glinr-dialog-pad)',
        rich: 'inset-x-0 bottom-0 max-h-[90dvh] w-full overflow-y-auto rounded-b-none pb-[max(var(--glinr-dialog-pad),env(safe-area-inset-bottom))] sm:inset-x-auto sm:bottom-auto sm:top-1/2 sm:left-1/2 sm:max-h-[85vh] sm:max-w-(--glinr-dialog-w-rich) sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-b-container sm:pb-(--glinr-dialog-pad)',
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
        fullscreen:
          'inset-0 w-auto max-w-none overflow-y-auto rounded-none pb-[max(var(--glinr-dialog-pad),env(safe-area-inset-bottom))] sm:inset-6 sm:rounded-container sm:pb-(--glinr-dialog-pad)',
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
  glass = true,
  ...props
}: DialogPrimitive.Popup.Props &
  VariantProps<typeof dialogContentVariants> & {
    showCloseButton?: boolean
    /** Glass is the default; pass false for a solid surface. */
    glass?: boolean
  }) {
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Popup
        data-slot="dialog-content"
        className={cn(
          dialogContentVariants({ size, className }),
          !glass && 'glass-solid',
        )}
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
            <XGlassIcon size="1.4em" />
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
        '-mx-(--glinr-dialog-pad) -mb-(--glinr-dialog-pad) flex flex-col-reverse gap-2 rounded-b-container border-t bg-muted/50 p-(--glinr-dialog-pad) sm:flex-row sm:justify-end',
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
