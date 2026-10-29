import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  GitBranchGlassIcon,
  GitForkGlassIcon,
  LockGlassIcon,
  ScaleGlassIcon,
  StarGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { BrandTile } from '../../BrandLogoBadge'
import { hostBrand } from '../../../lib/repoImport'
import type { PreviewLanguage, RepoPreview } from '../../../queries/repoPreview'

const SEGMENT_FILL = [
  'fill-sky-500',
  'fill-emerald-500',
  'fill-amber-500',
  'fill-violet-500',
  'fill-rose-500',
  'fill-slate-400',
]
const SEGMENT_BG = [
  'bg-sky-500',
  'bg-emerald-500',
  'bg-amber-500',
  'bg-violet-500',
  'bg-rose-500',
  'bg-slate-400',
]

function relativeTime(iso: string, locale: string): string {
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  const mins = Math.round((then - Date.now()) / 60_000)
  if (Math.abs(mins) < 60) return rtf.format(mins, 'minute')
  const hours = Math.round(mins / 60)
  if (Math.abs(hours) < 48) return rtf.format(hours, 'hour')
  const days = Math.round(hours / 24)
  if (Math.abs(days) < 60) return rtf.format(days, 'day')
  return rtf.format(Math.round(days / 30), 'month')
}

function LanguageBar({ languages }: { languages: PreviewLanguage[] }) {
  const { t } = useTranslation('newApp')
  return (
    <div className="space-y-2">
      <svg
        viewBox="0 0 100 1"
        preserveAspectRatio="none"
        role="img"
        aria-label={t('import.card.languages')}
        className="h-1.5 w-full overflow-hidden rounded-full"
      >
        {languages.map((l, i) => {
          return (
            <rect
              key={l.name}
              x={languages
                .slice(0, i)
                .reduce((sum, prev) => sum + prev.percent, 0)}
              y={0}
              width={l.percent}
              height={1}
              className={SEGMENT_FILL[i % SEGMENT_FILL.length]}
            />
          )
        })}
      </svg>
      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
        {languages.map((l, i) => (
          <li key={l.name} className="flex min-w-0 items-center gap-1.5">
            <span
              aria-hidden="true"
              className={`size-2 shrink-0 rounded-full ${SEGMENT_BG[i % SEGMENT_BG.length]}`}
            />
            <span className="truncate text-foreground">{l.name}</span>
            {l.percent.toFixed(1)}%
          </li>
        ))}
      </ul>
    </div>
  )
}

export function PreviewSkeleton({ label }: { label: string }) {
  return (
    <div
      className="space-y-3 rounded-lg border border-border bg-card p-4"
      role="status"
      aria-live="polite"
    >
      <span className="sr-only">{label}</span>
      <div className="flex items-center gap-3">
        <Skeleton className="size-8 rounded-md" />
        <Skeleton className="h-5 w-48 max-w-full" />
      </div>
      <Skeleton className="h-4 w-full" />
      <Skeleton className="h-1.5 w-full rounded-full" />
      <Skeleton className="h-4 w-2/3" />
      <Skeleton className="h-16 w-full" />
    </div>
  )
}

/** The rich repository card: identity, languages, last commit, README, branch. */
export function RepoPreviewCard({
  preview,
  branch,
  children,
}: {
  preview: RepoPreview
  branch: string
  children?: ReactNode
}) {
  const { t, i18n } = useTranslation('newApp')
  const commit = preview.last_commit
  return (
    <article className="min-w-0 space-y-4 rounded-lg border border-border bg-card p-4">
      <header className="flex min-w-0 flex-wrap items-center gap-3">
        <BrandTile
          name={hostBrand(preview.host)}
          label={preview.host ?? ''}
          size="md"
        />
        <h3 className="min-w-0 flex-1 truncate text-base font-medium text-foreground">
          <span className="text-muted-foreground">{preview.owner}/</span>
          {preview.name}
        </h3>
        <Badge variant="outline" className="gap-1">
          {preview.visibility === 'private' ? (
            <LockGlassIcon aria-hidden="true" className="size-4.5" />
          ) : null}
          {t(`import.card.visibility.${preview.visibility ?? 'public'}`)}
        </Badge>
      </header>
      {preview.description ? (
        <p className="text-sm text-foreground">{preview.description}</p>
      ) : null}
      {preview.languages && preview.languages.length > 0 ? (
        <LanguageBar languages={preview.languages} />
      ) : null}
      <ul className="flex flex-wrap gap-2 text-xs text-muted-foreground">
        {preview.stars !== undefined ? (
          <Chip icon={<StarGlassIcon aria-hidden="true" className="size-5" />}>
            {preview.stars.toLocaleString(i18n.language)}
          </Chip>
        ) : null}
        {preview.forks !== undefined ? (
          <Chip
            icon={<GitForkGlassIcon aria-hidden="true" className="size-5" />}
          >
            {preview.forks.toLocaleString(i18n.language)}
          </Chip>
        ) : null}
        {branch !== '' ? (
          <Chip
            icon={<GitBranchGlassIcon aria-hidden="true" className="size-5" />}
          >
            <span className="sr-only">{t('import.card.branch')}: </span>
            {branch}
          </Chip>
        ) : null}
        {preview.license ? (
          <Chip icon={<ScaleGlassIcon aria-hidden="true" className="size-5" />}>
            {preview.license}
          </Chip>
        ) : null}
      </ul>
      {commit ? (
        <p className="flex min-w-0 flex-wrap items-baseline gap-x-2 text-xs text-muted-foreground">
          <code className="font-mono text-foreground">{commit.sha}</code>
          <span className="min-w-0 truncate text-foreground">
            {commit.message}
          </span>
          <span>
            {t('import.card.commitBy', {
              author: commit.author,
              when: relativeTime(commit.at, i18n.language),
            })}
          </span>
        </p>
      ) : null}
      {preview.readme_excerpt ? (
        <p className="line-clamp-4 rounded-md bg-muted/50 p-3 text-xs text-muted-foreground">
          {preview.readme_excerpt}
        </p>
      ) : null}
      {children}
    </article>
  )
}

function Chip({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <li className="flex items-center gap-1 rounded-md border border-border px-2 py-0.5">
      {icon}
      {children}
    </li>
  )
}
