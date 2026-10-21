import { CommandBlock } from '../ui/code-block'

/** Labelled one-line command with copy; delegates to the shared CommandBlock. */
export function CopyCommand({
  label,
  command,
}: {
  label: string
  command: string
}) {
  return (
    <div className="space-y-1.5">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <CommandBlock code={command} label={label} />
    </div>
  )
}
