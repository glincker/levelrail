export interface KbdProps {
  keys: string[]
}

export function Kbd({ keys }: KbdProps) {
  return (
    <span className="inline-flex items-center gap-1">
      {keys.map((k, i) => (
        <kbd
          key={`${k}-${i}`}
          className="inline-flex h-5 min-w-5 items-center justify-center rounded-md border border-border bg-muted px-1.5 font-sans text-[11px] font-medium text-muted-foreground shadow-raised"
        >
          {k}
        </kbd>
      ))}
    </span>
  )
}
