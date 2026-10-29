import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useDatabaseWho } from '../../queries/databaseAccess'
import { GrantDialog } from './GrantDialog'

/** WhoCanAccessCard shows the platform users and tokens with abilities on this database and offers a scoped grant. */
export function WhoCanAccessCard({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('databaseAccess')
  const { data, isLoading } = useDatabaseWho(databaseName)
  const [granting, setGranting] = useState(false)
  const principals = data?.principals ?? []
  const policies = data?.policies ?? []
  const columns: ResponsiveColumn<(typeof principals)[number]>[] = [
    {
      id: 'who',
      header: t('who.columns.who'),
      primary: true,
      cell: (p) => (
        <div className="text-sm">
          {p.name}
          <div className="text-xs font-normal text-muted-foreground">
            {t(`who.type.${p.type}`)}
          </div>
        </div>
      ),
    },
    {
      id: 'abilities',
      header: t('who.columns.abilities'),
      cell: (p) => (
        <div className="flex flex-wrap gap-1">
          {p.effective.map((a) => (
            <Badge key={a} variant="muted">
              {a}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      id: 'via',
      header: t('who.columns.via'),
      priority: 2,
      cell: (p) => <span className="text-xs">{p.via.join(', ')}</span>,
    },
  ]

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-1.5 text-sm">
            <ShieldCheckGlassIcon
              className="size-5 text-tone-success"
              aria-hidden="true"
            />
            {t('who.title')}
          </CardTitle>
          <p className="text-xs text-muted-foreground">
            {t('who.description')}
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            render={<Link to="/settings/iam-policies" />}
          >
            {t('who.policies')}
          </Button>
          <Button size="sm" onClick={() => setGranting(true)}>
            {t('who.grant')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {isLoading ? null : principals.length === 0 ? (
          <EmptyState
            icon={<ShieldCheckGlassIcon className="size-6 text-tone-success" />}
            title={t('who.emptyTitle')}
            description={t('who.emptyBody')}
          />
        ) : (
          <ResponsiveTable
            rows={principals}
            columns={columns}
            getRowId={(p) => `${p.type}:${p.id}`}
            ariaLabel={t('who.tableLabel')}
          />
        )}
        {policies.length > 0 ? (
          <div className="space-y-1">
            <h3 className="text-xs font-medium">{t('who.policiesTitle')}</h3>
            <ul className="space-y-1 text-xs">
              {policies.map((p) => (
                <li key={p.id} className="flex flex-wrap items-center gap-2">
                  <span className="font-mono">{p.name}</span>
                  <Badge variant={p.scoped_to_database ? 'success' : 'warning'}>
                    {p.scoped_to_database
                      ? t('who.scopedOne')
                      : t('who.scopedWide')}
                  </Badge>
                  <span className="text-muted-foreground">
                    {t('who.attached', { count: p.principals.length })}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </CardContent>
      <GrantDialog
        databaseName={databaseName}
        open={granting}
        onOpenChange={setGranting}
        templates={data?.grant_templates ?? []}
      />
    </Card>
  )
}
