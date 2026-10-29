import { useTranslation } from 'react-i18next'
import {
  BookOpenGlassIcon,
  CheckGlassIcon,
  CopyGlassIcon,
  FlaskConicalGlassIcon,
  MapGlassIcon,
  SocialGithubGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { useDemoInfo } from '@/hooks/useDemo'
import { sendDemoCta } from '@/queries/demo'

function resetsLabel(iso: string | undefined): string | null {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  return new Intl.DateTimeFormat(undefined, {
    hour: 'numeric',
    minute: '2-digit',
  }).format(d)
}

/** DemoBanner is the compact, always-visible strip shown only on a demo instance. */
export function DemoBanner({ onTour }: { onTour: () => void }) {
  const { t } = useTranslation('demo')
  const info = useDemoInfo()
  const { copied, copy } = useCopyToClipboard()
  const resets = resetsLabel(info.resetsAt)

  return (
    <div
      role="status"
      className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-border bg-muted px-4 py-1.5 text-xs text-foreground"
    >
      <span className="flex items-center gap-1.5 font-medium">
        <FlaskConicalGlassIcon className="size-4" aria-hidden="true" />
        {t('banner.label')}
      </span>
      {resets ? (
        <span className="text-muted-foreground">
          {t('banner.resets', { time: resets })}
        </span>
      ) : null}
      <span className="ml-auto flex flex-wrap items-center gap-1.5">
        {info.installCommand ? (
          <Button
            type="button"
            variant="outline"
            size="xs"
            className="max-w-full font-mono"
            aria-label={t('banner.copyInstall')}
            title={t('banner.copyInstall')}
            onClick={() => {
              copy(info.installCommand ?? '')
              sendDemoCta('install')
            }}
          >
            <span className="truncate">{info.installCommand}</span>
            {copied ? (
              <CheckGlassIcon size="1.2em" data-icon="inline-end" />
            ) : (
              <CopyGlassIcon size="1.2em" data-icon="inline-end" />
            )}
            <span className="sr-only">
              {copied ? t('banner.copied') : t('banner.copyInstall')}
            </span>
          </Button>
        ) : null}
        {info.repoUrl ? (
          <Button
            variant="ghost"
            size="xs"
            nativeButton={false}
            render={
              <a
                href={info.repoUrl}
                target="_blank"
                rel="noreferrer"
                onClick={() => {
                  sendDemoCta('github')
                }}
              />
            }
          >
            <SocialGithubGlassIcon size="1.2em" data-icon="inline-start" />
            {t('banner.github')}
          </Button>
        ) : null}
        {info.docsUrl ? (
          <Button
            variant="ghost"
            size="xs"
            nativeButton={false}
            render={
              <a
                href={info.docsUrl}
                target="_blank"
                rel="noreferrer"
                onClick={() => {
                  sendDemoCta('docs')
                }}
              />
            }
          >
            <BookOpenGlassIcon size="1.2em" data-icon="inline-start" />
            {t('banner.docs')}
          </Button>
        ) : null}
        <Button type="button" variant="ghost" size="xs" onClick={onTour}>
          <MapGlassIcon size="1.2em" data-icon="inline-start" />
          {t('banner.tour')}
        </Button>
      </span>
    </div>
  )
}
