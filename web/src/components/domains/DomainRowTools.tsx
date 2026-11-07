import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightGlassIcon,
  SettingsGlassIcon,
  TrashGlassIcon,
  UnlinkGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type { TargetKind } from '../../queries/parkedDomains'
import {
  DeleteDomainDialog,
  MoveDomainDialog,
  ParkDomainDialog,
} from './DomainLifecycleDialogs'

type Tool = 'park' | 'move' | 'delete'

// Compact action row for one saved domain on the per-app page. In an
// environment tab (environment set) only park and move apply, the row's own
// remove button covers delete.
export function DomainRowTools({
  appName,
  domain,
  environment,
  kind = 'app',
}: {
  appName: string
  domain: string
  environment?: string
  kind?: TargetKind
}) {
  const scoped = environment !== undefined
  // Static sites keep no per-domain settings, so there is no details sheet.
  const hideDetails = scoped || kind === 'static_site'
  const { t } = useTranslation('domains')
  const [tool, setTool] = useState<Tool | null>(null)
  const close = (open: boolean) => {
    if (!open) setTool(null)
  }
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {hideDetails ? null : (
        <Button
          type="button"
          size="sm"
          variant="outline"
          render={
            <Link
              to="/apps/$name/domains/$domain"
              params={{ name: appName, domain }}
            />
          }
          nativeButton={false}
        >
          <SettingsGlassIcon size="1.4em" aria-hidden="true" />
          {t('page.row.details')}
        </Button>
      )}
      <Button
        type="button"
        size="sm"
        variant="ghost"
        onClick={() => {
          setTool('move')
        }}
      >
        <ArrowRightGlassIcon size="1.4em" aria-hidden="true" />
        {t('page.row.move')}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        onClick={() => {
          setTool('park')
        }}
      >
        <UnlinkGlassIcon size="1.4em" aria-hidden="true" />
        {t('page.row.park')}
      </Button>
      {scoped ? null : (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => {
            setTool('delete')
          }}
        >
          <TrashGlassIcon size="1.4em" aria-hidden="true" />
          {t('page.row.delete')}
        </Button>
      )}
      {tool === 'park' ? (
        <ParkDomainDialog
          app={appName}
          domain={domain}
          environment={environment}
          kind={kind}
          open
          onOpenChange={close}
        />
      ) : null}
      {tool === 'move' ? (
        <MoveDomainDialog
          from={appName}
          domain={domain}
          fromEnvironment={environment}
          fromKind={kind}
          open
          onOpenChange={close}
        />
      ) : null}
      {tool === 'delete' ? (
        <DeleteDomainDialog
          app={appName}
          domain={domain}
          open
          onOpenChange={close}
        />
      ) : null}
    </div>
  )
}
