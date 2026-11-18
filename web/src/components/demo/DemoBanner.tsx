import { useTranslation } from 'react-i18next'
import {
  SocialGithubGlassIcon,
  StarGlassIcon,
  TerminalGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useDemoInfo } from '@/hooks/useDemo'
import { captureDemoEvent } from '@/lib/demoAnalytics'
import { formatStars } from '@/lib/demoStars'
import { sendDemoCta } from '@/queries/demo'

export const DEFAULT_REPO_URL = 'https://github.com/glincker/levelrail'

function resetsLabel(iso: string | undefined): string | null {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  return new Intl.DateTimeFormat(undefined, {
    hour: 'numeric',
    minute: '2-digit',
  }).format(d)
}

/** DemoStarButton is the high-contrast GitHub call to action; the count is omitted when the API has none. */
export function DemoStarButton() {
  const { t } = useTranslation('demo')
  const info = useDemoInfo()
  const count = formatStars(info.stars)
  return (
    <Button
      size="xs"
      className="bg-foreground text-background hover:bg-foreground/85 focus-visible:ring-ring"
      nativeButton={false}
      render={
        <a
          href={info.repoUrl || DEFAULT_REPO_URL}
          target="_blank"
          rel="noopener"
          aria-label={
            count ? t('banner.starCount', { count }) : t('banner.starLabel')
          }
          onClick={() => {
            sendDemoCta('github')
            captureDemoEvent('demo_github_click')
          }}
        />
      }
    >
      <SocialGithubGlassIcon size="1.4em" data-icon="inline-start" />
      <StarGlassIcon size="1.2em" data-icon="inline-start" />
      <span className="hidden sm:inline">{t('banner.star')}</span>
      {count ? (
        <span className="border-l border-background/30 pl-1.5 tabular-nums">
          {count}
        </span>
      ) : null}
    </Button>
  )
}

/** DemoBanner is one slim line plus the Star and Install calls to action; shown only on a demo instance. */
export function DemoBanner({ onInstall }: { onInstall: () => void }) {
  const { t } = useTranslation('demo')
  const info = useDemoInfo()
  const resets = resetsLabel(info.resetsAt)

  return (
    <div
      role="status"
      className="flex shrink-0 items-center gap-2 border-b border-border bg-muted/60 px-3 py-1 text-xs text-muted-foreground"
    >
      <span className="min-w-0 flex-1 truncate">
        {resets
          ? t('banner.message', { time: resets })
          : t('banner.messageNoReset')}
      </span>
      <span className="flex shrink-0 items-center gap-1.5">
        <DemoStarButton />
        <Button
          type="button"
          size="xs"
          variant="outline"
          aria-label={t('banner.install')}
          onClick={onInstall}
        >
          <TerminalGlassIcon size="1.4em" data-icon="inline-start" />
          <span className="hidden sm:inline">{t('banner.install')}</span>
        </Button>
      </span>
    </div>
  )
}
