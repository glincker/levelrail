import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LinkGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useParkedDomains } from '../../queries/parkedDomains'
import {
  AttachParkedDialog,
  DeleteParkedDialog,
} from './DomainLifecycleDialogs'

type Action = { kind: 'attach' | 'delete'; domain: string }

// Parked domains stay registered with DNS and certificate but route to no app.
export function ParkedDomainsGroup() {
  const { t } = useTranslation('domains')
  const { data: parked = [] } = useParkedDomains()
  const [action, setAction] = useState<Action | null>(null)
  if (parked.length === 0) return null
  return (
    <section aria-labelledby="parked-domains-heading" className="space-y-2">
      <div className="flex items-center gap-2">
        <h2 id="parked-domains-heading" className="text-sm font-medium">
          {t('parked.title')}
        </h2>
        <Badge variant="muted">{parked.length}</Badge>
      </div>
      <p className="text-xs text-muted-foreground">{t('parked.hint')}</p>
      <ul className="divide-y divide-border rounded-lg border border-border">
        {parked.map((p) => (
          <li
            key={p.domain}
            className="flex flex-wrap items-center gap-2 px-3 py-2"
          >
            <span className="min-w-0 flex-1 truncate font-mono text-sm">
              {p.domain}
            </span>
            {p.parked_from ? (
              <span className="text-xs text-muted-foreground">
                {t('parked.from', { app: p.parked_from })}
              </span>
            ) : null}
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setAction({ kind: 'attach', domain: p.domain })
              }}
            >
              <LinkGlassIcon size="1.4em" aria-hidden="true" />
              {t('parked.attach')}
            </Button>
            <Button
              size="icon-sm"
              variant="ghost"
              onClick={() => {
                setAction({ kind: 'delete', domain: p.domain })
              }}
            >
              <TrashGlassIcon size="1.4em" aria-hidden="true" />
              <span className="sr-only">
                {t('parked.delete', { domain: p.domain })}
              </span>
            </Button>
          </li>
        ))}
      </ul>
      {action?.kind === 'attach' ? (
        <AttachParkedDialog
          domain={action.domain}
          open
          onOpenChange={(o) => {
            if (!o) setAction(null)
          }}
        />
      ) : null}
      {action?.kind === 'delete' ? (
        <DeleteParkedDialog
          domain={action.domain}
          open
          onOpenChange={(o) => {
            if (!o) setAction(null)
          }}
        />
      ) : null}
    </section>
  )
}
