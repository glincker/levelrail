import {
  CircleCheckGlassIcon,
  CircleUserGlassIcon,
  GitBranchGlassIcon,
  LockGlassIcon,
} from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { NormalizedRepoOption } from '../lib/gitRepoOptions'
import { BrandIcon, isBrandIconName } from './BrandIcon'

// The owner is its own dimmed prefix so two repos with one name under
// different owners are told apart at a glance.
function splitPath(displayPath: string): { owner: string; name: string } {
  const slash = displayPath.lastIndexOf('/')
  if (slash < 0) return { owner: '', name: displayPath }
  return {
    owner: displayPath.slice(0, slash + 1),
    name: displayPath.slice(slash + 1),
  }
}

export function GitRepoCard({
  option,
  selected,
  runningAs,
  onSelect,
}: Readonly<{
  option: NormalizedRepoOption
  selected: boolean
  runningAs?: string
  onSelect: () => void
}>) {
  const { t } = useTranslation('gitRepos')
  const { owner, name } = splitPath(option.displayPath)
  return (
    <button
      type="button"
      aria-pressed={selected}
      onClick={onSelect}
      className={cn(
        'flex flex-col items-start gap-1.5 rounded-lg border border-border bg-card p-2.5 text-left transition-colors hover:border-primary/40 hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
        selected && 'border-primary/50 bg-muted',
      )}
    >
      <span className="flex w-full items-center gap-1.5 text-sm font-medium text-foreground">
        <span className="truncate">
          <span className="text-muted-foreground">{owner}</span>
          {name}
        </span>
        {option.private ? (
          <LockGlassIcon
            className="size-5 shrink-0 text-muted-foreground"
            aria-label={t('picker.badges.private')}
          />
        ) : null}
      </span>
      {option.account || option.connectionLabel ? (
        <span className="flex w-full items-center gap-1 text-xs text-muted-foreground">
          {isBrandIconName(option.providerRef.kind) ? (
            <BrandIcon
              name={option.providerRef.kind}
              className="size-3 shrink-0"
            />
          ) : (
            <CircleUserGlassIcon
              className="size-4.5 shrink-0"
              aria-hidden="true"
            />
          )}
          <span className="truncate">
            {t('picker.account', {
              name: option.account || option.connectionLabel,
            })}
          </span>
        </span>
      ) : null}
      <span className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
        <GitBranchGlassIcon className="size-4.5 shrink-0" aria-hidden="true" />
        <span className="truncate font-mono">{option.defaultBranch}</span>
        {option.archived ? (
          <Badge variant="muted">{t('picker.badges.archived')}</Badge>
        ) : null}
        {option.fork ? (
          <Badge variant="muted">{t('picker.badges.fork')}</Badge>
        ) : null}
      </span>
      {runningAs ? (
        <span className="flex items-center gap-1 text-xs font-medium text-primary">
          <CircleCheckGlassIcon
            className="size-5 shrink-0 text-tone-success"
            aria-hidden="true"
          />
          <span className="truncate">
            {t('picker.badges.running', { name: runningAs })}
          </span>
        </span>
      ) : null}
    </button>
  )
}
