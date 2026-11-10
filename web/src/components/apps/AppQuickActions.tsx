import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ExternalLinkGlassIcon,
  RocketGlassIcon,
  TerminalGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useRedeployApp } from '../../hooks/useRedeployApp'
import type { AppListEntry } from '../../types/appDetail'

export function AppQuickActions({ app }: { app: AppListEntry }) {
  const { redeploy, isPending } = useRedeployApp(app.name, app.image)
  const { t } = useTranslation('common')
  const domain = app.domains?.[0]
  return (
    <span className="relative z-10 flex items-center gap-0.5 max-lg:hidden opacity-60 transition-opacity duration-150 group-focus-within/row:opacity-100 group-hover/row:opacity-100 [@media(pointer:coarse)]:[&>*]:size-10 motion-reduce:transition-none">
      {domain ? (
        <Button
          size="icon-sm"
          variant="ghost"
          aria-label={t('appsList.quick.open', { domain })}
          render={
            <a href={`https://${domain}`} target="_blank" rel="noreferrer" />
          }
          nativeButton={false}
        >
          <ExternalLinkGlassIcon size="1.4em" />
        </Button>
      ) : (
        // Holds the open-domain slot so the other icons line up across rows.
        <span
          aria-hidden="true"
          data-slot="quick-action-placeholder"
          className="inline-block size-7 shrink-0 pointer-coarse:size-11"
        />
      )}
      <Button
        size="icon-sm"
        variant="ghost"
        aria-label={t('appsList.quick.redeploy', { name: app.name })}
        disabled={isPending}
        onClick={redeploy}
      >
        <RocketGlassIcon size="1.4em" />
      </Button>
      <Button
        size="icon-sm"
        variant="ghost"
        aria-label={t('appsList.quick.logs', { name: app.name })}
        render={<Link to="/apps/$name/logs" params={{ name: app.name }} />}
        nativeButton={false}
      >
        <TerminalGlassIcon size="1.4em" />
      </Button>
    </span>
  )
}
