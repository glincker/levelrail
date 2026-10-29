import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { HelpLink } from '@/components/HelpLink'
import { domainsQueryOptions, type Domain } from '../../queries/domains'
import type { DomainTab } from '../domain/domainTabs'
import { CdnFrame } from './CdnFrame'

type Kind = 'redirects' | 'routing'

const TABS: Record<
  Kind,
  Extract<DomainTab, 'redirects' | 'forwarding' | 'headers'>[]
> = {
  redirects: ['redirects'],
  routing: ['forwarding', 'headers'],
}

/**
 * CdnControlsPage is the Redirects and Routing rules page of the CDN section.
 * Both controls already live on each domain's page, so this lists the domains
 * and links straight to the right tab instead of building a second editor.
 */
export function CdnControlsPage({ kind }: Readonly<{ kind: Kind }>) {
  const { t } = useTranslation('cdn')
  const { data, error, isPending } = useQuery(domainsQueryOptions())
  const rows = (data ?? []).filter((d) => d.service_name !== '')

  const columns: ResponsiveColumn<Domain>[] = [
    {
      id: 'domain',
      header: t('controls.domain'),
      primary: true,
      cell: (d) => (
        <span
          className="block max-w-72 truncate font-mono text-xs"
          title={d.domain}
        >
          {d.domain}
        </span>
      ),
    },
    {
      id: 'app',
      header: t('controls.app'),
      priority: 2,
      cell: (d) => d.service_name,
    },
    {
      id: 'open',
      header: <span className="sr-only">{t('controls.open')}</span>,
      align: 'end',
      cell: (d) => (
        <span className="flex flex-wrap justify-end gap-2">
          {TABS[kind].map((tab) => (
            <Link
              key={tab}
              to="/domains/$domain"
              params={{ domain: d.domain }}
              search={{ app: d.service_name, tab }}
              className="text-sm underline underline-offset-4"
            >
              {t(`controls.tab.${tab}`)}
            </Link>
          ))}
        </span>
      ),
    },
  ]

  return (
    <CdnFrame
      title={t(`controls.${kind}.title`)}
      description={t(`controls.${kind}.description`)}
      actions={
        <HelpLink
          path="/domain-traffic-controls"
          label={t('controls.docs')}
          variant="inline"
        />
      }
    >
      {isPending ? <TableSkeleton columnCount={3} rowCount={4} /> : null}
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : null}
      {data && rows.length === 0 ? (
        <EmptyState
          icon={<GlobeGlassIcon className="size-5.5" />}
          title={t('controls.emptyTitle')}
          description={t('controls.emptyBody')}
          action={
            <Button render={<Link to="/domains" />} nativeButton={false}>
              {t('controls.emptyAction')}
            </Button>
          }
        />
      ) : null}
      {rows.length > 0 ? (
        <ResponsiveTable
          rows={rows}
          columns={columns}
          getRowId={(d) => d.domain}
          ariaLabel={t(`controls.${kind}.aria`)}
        />
      ) : null}
    </CdnFrame>
  )
}
