import { cn } from '@/lib/utils'

// Opaque tokens and an explicit color-scheme keep the native option list
// readable in dark mode, where a transparent select shows a white list.
function NativeSelect({ className, ...props }: React.ComponentProps<'select'>) {
  return (
    <select
      data-slot="native-select"
      className={cn(
        'h-8 rounded-lg border border-input bg-background px-2 text-sm text-foreground [color-scheme:light] focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 pointer-coarse:h-11 dark:bg-input/30 dark:[color-scheme:dark] [&_option]:bg-popover [&_option]:text-popover-foreground',
        className,
      )}
      {...props}
    />
  )
}

export { NativeSelect }
