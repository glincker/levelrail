import { LockGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import type { NormalizedRepoOption } from '../lib/gitRepoOptions'

export function GitSelectedRepoBar({
  option,
  branch,
  branchOptions,
  branchesLoading,
  branchError,
  disabled,
  idPrefix,
  onBranchChange,
  onChange,
}: Readonly<{
  option: NormalizedRepoOption
  branch: string
  branchOptions: { value: string; label: string }[]
  branchesLoading: boolean
  branchError?: string
  disabled?: boolean
  idPrefix: string
  onBranchChange: (branch: string) => void
  onChange: () => void
}>) {
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-3 rounded-lg border border-primary/40 bg-muted/50 p-3">
        <div className="flex min-w-0 flex-1 items-center gap-1.5 text-sm font-medium text-foreground">
          <span className="truncate">{option.displayPath}</span>
          {option.private ? (
            <LockGlassIcon
              className="size-4 shrink-0 text-muted-foreground"
              aria-label="Private repository"
            />
          ) : null}
        </div>
        <div className="w-full sm:w-56">
          <Combobox
            id={`${idPrefix}-branch`}
            options={branchOptions}
            value={branch}
            isLoading={branchesLoading}
            disabled={disabled}
            placeholder="Select a branch"
            searchPlaceholder="Search branches..."
            triggerClassName="font-mono"
            onValueChange={onBranchChange}
          />
        </div>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled}
          onClick={onChange}
        >
          Change
        </Button>
      </div>
      {branchError ? (
        <p className="text-sm text-destructive">{branchError}</p>
      ) : null}
    </div>
  )
}
