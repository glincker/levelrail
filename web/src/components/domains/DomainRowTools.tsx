import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightGlassIcon,
  SettingsGlassIcon,
  TrashGlassIcon,
  UnlinkGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { DomainDetailSheet } from './DomainDetailSheet'
import {
  DeleteDomainDialog,
  MoveDomainDialog,
  ParkDomainDialog,
} from './DomainLifecycleDialogs'

type Tool = 'details' | 'park' | 'move' | 'delete'

// Compact action row for one saved domain on the per-app page. In an
// environment tab (environment set) only park and move apply, the row's own
// remove button covers delete.
export function DomainRowTools({
  appName,
  domain,
  environment,
}: {
  appName: string
  domain: string
  environment?: string
}) {
  const scoped = environment !== undefined
  const { t } = useTranslation('domains')
  const [tool, setTool] = useState<Tool | null>(null)
  const close = (open: boolean) => {
    if (!open) setTool(null)
  }
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {scoped ? null : (
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            setTool('details')
          }}
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
      {tool === 'details' ? (
        <DomainDetailSheet
          appName={appName}
          domain={domain}
          open
          onOpenChange={close}
        />
      ) : null}
      {tool === 'park' ? (
        <ParkDomainDialog
          app={appName}
          domain={domain}
          environment={environment}
          open
          onOpenChange={close}
        />
      ) : null}
      {tool === 'move' ? (
        <MoveDomainDialog
          from={appName}
          domain={domain}
          fromEnvironment={environment}
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
