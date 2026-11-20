import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleXGlassIcon,
  LoaderGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import type { GitStep } from './pagesGitLogic'

/** Four-step build progress with real per-step states; unknown stays neutral like pending. */
export function GitSteps({ steps }: Readonly<{ steps: GitStep[] }>) {
  const { t } = useTranslation('pagesSites')
  return (
    <ol
      aria-label={t('git.steps.label')}
      className="grid grid-cols-2 gap-2 min-[480px]:grid-cols-4"
    >
      {steps.map((s) => (
        <li
          key={s.name}
          data-state={s.state}
          className={cn(
            'flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs',
            s.state === 'failed'
              ? 'border-destructive bg-destructive/10 text-destructive'
              : 'border-border text-muted-foreground',
            s.state === 'done' && 'text-foreground',
            s.state === 'running' && 'border-ring text-foreground',
          )}
        >
          {s.state === 'done' ? (
            <CheckGlassIcon className="size-3.5 shrink-0" aria-hidden="true" />
          ) : s.state === 'failed' ? (
            <CircleXGlassIcon
              className="size-3.5 shrink-0"
              aria-hidden="true"
            />
          ) : s.state === 'running' ? (
            <LoaderGlassIcon
              className="size-3.5 shrink-0 motion-safe:animate-spin"
              aria-hidden="true"
            />
          ) : (
            <span
              className="size-3.5 shrink-0 rounded-full border border-border"
              aria-hidden="true"
            />
          )}
          <span>{t(`git.steps.${s.name}`)}</span>
          <span className="sr-only">{t(`git.steps.state.${s.state}`)}</span>
        </li>
      ))}
    </ol>
  )
}
