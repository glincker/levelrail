import { useTranslation } from 'react-i18next'
import { ArrowSquareOutIcon, GlobeIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { useAppNetwork } from '../queries/appNetwork'

/** AutomaticDomainRow shows the zero-config hostname Levelrail assigned, so the Domains page agrees with the Overview. */
export function AutomaticDomainRow({ appName }: { appName: string }) {
  const { t } = useTranslation('networkProxy')
  const { data: network } = useAppNetwork(appName)
  const url = network?.fallback_url
  if (!url) return null
  const host = url.replace(/^https?:\/\//, '').replace(/\/$/, '')

  return (
    <div className="space-y-1 rounded-md border border-border px-3 py-2">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <GlobeIcon
          className="size-4 text-muted-foreground"
          aria-hidden="true"
        />
        <span className="font-mono text-foreground">{host}</span>
        <Badge variant="muted">{t('automaticDomain.badge')}</Badge>
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          className="ml-auto inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          {t('automaticDomain.open')}
          <ArrowSquareOutIcon className="size-3.5" aria-hidden="true" />
        </a>
      </div>
      <p className="text-xs text-muted-foreground">
        {t('automaticDomain.hint')}
      </p>
    </div>
  )
}
