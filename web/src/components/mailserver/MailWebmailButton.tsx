import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon, MailGlassIcon } from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { appUrlsQueryOptions } from '../../queries/appUrls'
import { webmailUrl } from './mailSetup'

interface Props {
  app: string | undefined
  address?: string
  compact?: boolean
}

export function MailWebmailButton({ app, address, compact }: Props) {
  const { t } = useTranslation('mailServer')
  const q = useQuery({
    ...appUrlsQueryOptions(app ?? ''),
    enabled: !!app,
    refetchInterval: false,
  })
  const url = webmailUrl(q.data)
  const label = address ? t('webmail.openFor', { address }) : t('webmail.open')
  const text = compact ? t('webmail.open') : label
  if (url) {
    return (
      <a
        href={url}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={label}
        className={cn(
          buttonVariants({ variant: 'outline', size: 'sm' }),
          'gap-1.5',
        )}
      >
        <ExternalLinkGlassIcon size="1.1em" />
        {text}
      </a>
    )
  }
  return (
    <span
      className="inline-flex items-center gap-1"
      title={t('webmail.disabled')}
    >
      <Button
        variant="outline"
        size="sm"
        className="gap-1.5"
        disabled
        aria-label={label}
      >
        <MailGlassIcon size="1.1em" />
        {text}
      </Button>
      {app ? (
        <Link
          to="/apps/$name"
          params={{ name: app }}
          className="text-xs text-muted-foreground underline hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
        >
          {t('webmail.settings')}
        </Link>
      ) : null}
    </span>
  )
}
