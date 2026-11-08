import { Button as ButtonPrimitive } from '@base-ui/react/button'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'
import { isLinkRender, nativeButtonFromRender } from '@/lib/nativeButton'
import { useIsDemo } from '@/hooks/useDemo'
import { DemoBlockedTip } from '@/components/ui/demo-blocked-tip'

const buttonVariants = cva(
  "group/button inline-flex shrink-0 items-center justify-center relative rounded-(--glinr-radius-control) border border-transparent bg-clip-padding text-sm font-medium whitespace-nowrap transition-transform duration-(--glinr-dur-press) ease-(--glinr-ease-out) outline-none select-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-(--glinr-accent-ring) focus-visible:ring-offset-(--glinr-surface-bg) active:not-aria-[haspopup]:not-disabled:not-aria-busy:scale-(--glinr-press-scale) motion-reduce:transition-none motion-reduce:active:scale-100 disabled:pointer-events-none disabled:opacity-55 aria-busy:cursor-progress aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-5",
  {
    variants: {
      variant: {
        default:
          'bg-(--glinr-accent-solid) text-(--glinr-accent-ink) [box-shadow:var(--glinr-specular),var(--glinr-shadow-1)] hover:bg-(--glinr-accent-solid-hover) active:bg-(--glinr-accent-solid-active)',
        tinted:
          'bg-(--glinr-accent-soft) text-(--glinr-accent-fg) hover:bg-[color-mix(in_oklab,var(--glinr-accent-solid)_18%,transparent)]',
        'destructive-solid':
          'bg-(--glinr-status-danger-solid) text-(--glinr-accent-ink) [box-shadow:var(--glinr-specular)] hover:brightness-95 active:brightness-90',
        outline:
          'border-border bg-background hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground dark:border-input dark:bg-input/30 dark:hover:bg-input/50',
        secondary:
          'border-(--glinr-color-border-strong) bg-(--glinr-surface-raised) text-(--glinr-color-fg) [box-shadow:var(--glinr-specular),var(--glinr-shadow-1)] hover:bg-(--glinr-hover) active:bg-(--glinr-press) aria-expanded:bg-(--glinr-hover)',
        ghost:
          'text-(--glinr-color-fg-muted) hover:bg-(--glinr-hover) hover:text-(--glinr-color-fg) active:bg-(--glinr-press) aria-expanded:bg-(--glinr-hover) aria-expanded:text-(--glinr-color-fg)',
        destructive:
          'border-(--glinr-status-danger-border) bg-(--glinr-status-danger-soft) text-(--glinr-status-danger-fg) hover:bg-(--glinr-status-danger-border) active:bg-(--glinr-status-danger-border)',
        link: 'text-brand underline-offset-4 hover:underline',
        // Brand gradient-stroke CTA (adr/024): same two-layer background-image
        // recipe as .glinui-btn--primary in styles/glinui-tokens.css, written
        // as Tailwind utilities so it composes with the existing cva shape
        // instead of mixing class systems. Additive only, never the default.
        // glinui-bg-clip-split: Tailwind's bg-clip-*/bg-origin-* utilities
        // are enumerated keywords, not arbitrary-value utilities --
        // bg-clip-[padding-box,border-box] silently compiles to nothing
        // (confirmed against the built CSS), which breaks the padding-box/
        // border-box split this recipe depends on. Real CSS class instead.
        glinui:
          'glinui-bg-clip-split rounded-full border-[1.5px] bg-[image:linear-gradient(160deg,var(--glinui-accent-soft-text)_0%,var(--glinui-accent-strong)_100%),linear-gradient(180deg,var(--glinui-accent-soft-text)_0%,transparent_100%)] font-semibold text-white hover:brightness-110 active:scale-[0.98]',
      },
      size: {
        default:
          'h-(--glinr-control-h) gap-1.5 px-2.5 pointer-coarse:min-h-11 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2',
        xs: "h-6 gap-1 rounded-[min(var(--radius-md),10px)] px-2 text-xs in-data-[slot=button-group]:rounded-lg has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3.5",
        sm: "h-7 pointer-coarse:min-h-11 gap-1 rounded-[min(var(--radius-md),12px)] px-2.5 text-[0.8rem] in-data-[slot=button-group]:rounded-lg has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-4.5",
        lg: 'h-(--glinr-capsule-h-lg) pointer-coarse:min-h-11 gap-1.5 px-2.5 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2',
        icon: 'size-(--glinr-control-h) pointer-coarse:size-11',
        'icon-xs':
          "size-6 rounded-[min(var(--radius-md),10px)] in-data-[slot=button-group]:rounded-lg [&_svg:not([class*='size-'])]:size-3.5",
        'icon-sm':
          'size-7 pointer-coarse:size-11 rounded-[min(var(--radius-md),12px)] in-data-[slot=button-group]:rounded-lg',
        'icon-lg': 'size-9 pointer-coarse:size-11',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)

type ButtonProps = ButtonPrimitive.Props &
  VariantProps<typeof buttonVariants> & {
    /** Marks a control that mutates state: rendered disabled with an explanation in the read-only demo. */
    writeAction?: boolean
    /** Keeps the label so the width never jumps; the control stays focusable but inert. */
    loading?: boolean
  }

function Button({
  className,
  variant = 'default',
  size = 'default',
  writeAction = false,
  loading = false,
  children,
  ...props
}: ButtonProps) {
  const demo = useIsDemo()
  const blocked = writeAction && demo
  // Base UI stamps role="button" on non-native renders; a link must stay a link for assistive tech.
  const role = props.role ?? (isLinkRender(props.render) ? 'link' : undefined)
  const node = (
    <ButtonPrimitive
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
      onClick={loading ? undefined : props.onClick}
      aria-busy={loading || undefined}
      aria-disabled={loading || props['aria-disabled']}
      role={role}
      nativeButton={nativeButtonFromRender(props.render, props.nativeButton)}
      disabled={blocked ? true : props.disabled}
    >
      {loading ? (
        <>
          <span className="invisible inline-flex items-center gap-[inherit]">
            {children}
          </span>
          <svg
            aria-hidden="true"
            viewBox="0 0 16 16"
            className="absolute size-4 animate-spin fill-none stroke-current stroke-2 motion-reduce:animate-none"
          >
            <path d="M8 2a6 6 0 1 0 6 6" strokeLinecap="round" />
          </svg>
        </>
      ) : (
        children
      )}
    </ButtonPrimitive>
  )
  return blocked ? <DemoBlockedTip>{node}</DemoBlockedTip> : node
}

export { Button, buttonVariants }
