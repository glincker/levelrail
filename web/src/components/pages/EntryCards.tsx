import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  FolderGlassIcon,
  GitBranchGlassIcon,
  LayoutTemplateGlassIcon,
} from '@thesvg/ui-react'

const CARD =
  'flex flex-1 basis-56 items-center gap-3 rounded-lg border border-border bg-card px-4 py-3 text-left transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring'

function Body({
  icon,
  title,
  body,
}: Readonly<{ icon: ReactNode; title: string; body: string }>) {
  return (
    <>
      <span className="text-muted-foreground">{icon}</span>
      <span className="min-w-0">
        <span className="block text-sm font-medium">{title}</span>
        <span className="block text-xs text-muted-foreground">{body}</span>
      </span>
    </>
  )
}

/** Three ways to start the first site, shown instead of an empty table. */
export function EntryCards({
  onDrop,
  onBlank,
}: Readonly<{ onDrop: () => void; onBlank: () => void }>) {
  const { t } = useTranslation('pagesSites')
  return (
    <div className="flex flex-wrap gap-3">
      <button type="button" className={CARD} onClick={onDrop}>
        <Body
          icon={<FolderGlassIcon size="1.6em" aria-hidden="true" />}
          title={t('entry.drop.title')}
          body={t('entry.drop.body')}
        />
      </button>
      <Link to="/apps/new" className={CARD}>
        <Body
          icon={<GitBranchGlassIcon size="1.6em" aria-hidden="true" />}
          title={t('entry.git.title')}
          body={t('entry.git.body')}
        />
      </Link>
      <button type="button" className={CARD} onClick={onBlank}>
        <Body
          icon={<LayoutTemplateGlassIcon size="1.6em" aria-hidden="true" />}
          title={t('entry.blank.title')}
          body={t('entry.blank.body')}
        />
      </button>
    </div>
  )
}
