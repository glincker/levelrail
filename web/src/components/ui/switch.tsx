import { Switch as SwitchPrimitive } from '@base-ui/react/switch'

import { cn } from '@/lib/utils'

function Switch({
  className,
  size = 'default',
  ...props
}: SwitchPrimitive.Root.Props & {
  size?: 'sm' | 'default'
}) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      data-size={size}
      className={cn(
        'peer group/switch relative inline-flex shrink-0 items-center rounded-full border border-transparent transition-colors outline-none after:absolute after:-inset-x-3 after:-inset-y-2 focus-visible:ring-3 focus-visible:ring-(--glinr-focus-halo) focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 data-[size=default]:h-6 data-[size=default]:w-10 data-[size=sm]:h-[18px] data-[size=sm]:w-[30px] dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 data-checked:bg-(--glinr-accent-solid) data-unchecked:bg-muted-foreground/45 data-disabled:cursor-not-allowed data-disabled:opacity-50',
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb
        data-slot="switch-thumb"
        className="pointer-events-none block rounded-full bg-white shadow-[0_1px_1px_oklch(0_0_0/20%),0_2px_4px_oklch(0_0_0/20%)] ring-0 transition-transform motion-reduce:transition-none group-data-[size=default]/switch:size-5 group-data-[size=sm]/switch:size-[14px] group-data-[size=default]/switch:data-checked:translate-x-[18px] group-data-[size=sm]/switch:data-checked:translate-x-[14px] group-data-[size=default]/switch:data-unchecked:translate-x-0.5 group-data-[size=sm]/switch:data-unchecked:translate-x-0.5"
      />
    </SwitchPrimitive.Root>
  )
}

export { Switch }
