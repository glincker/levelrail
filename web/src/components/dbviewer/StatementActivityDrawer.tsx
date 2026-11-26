import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { useStatementAudit } from '../../queries/databaseViewer'

const ROWS = 50

/** Recent statements run against this database. Callers without the right never see the entry point. */
export function StatementActivityDrawer({
  databaseName,
  open,
  onOpenChange,
}: Readonly<{
  databaseName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}>) {
  const { t } = useTranslation('dbExplorerStrip')
  const audit = useStatementAudit(databaseName, ROWS, open)
  const items = audit.data?.items ?? []

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full gap-3 sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>{t('activity.title')}</SheetTitle>
          <SheetDescription>
            {t('activity.description', { count: ROWS })}
          </SheetDescription>
        </SheetHeader>
        <div className="min-h-0 flex-1 overflow-auto px-4 pb-4">
          {audit.isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : audit.isError ? (
            <div className="space-y-2" role="alert">
              <p className="text-sm text-destructive">{t('activity.error')}</p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  void audit.refetch()
                }}
              >
                {t('retry')}
              </Button>
            </div>
          ) : items.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('activity.empty')}
            </p>
          ) : (
            <table className="w-full text-left text-xs">
              <thead className="text-muted-foreground">
                <tr>
                  <th className="py-1 pr-3 font-medium">
                    {t('activity.time')}
                  </th>
                  <th className="py-1 pr-3 font-medium">
                    {t('activity.principal')}
                  </th>
                  <th className="py-1 pr-3 font-medium">
                    {t('activity.action')}
                  </th>
                  <th className="py-1 pr-3 font-medium">
                    {t('activity.outcome')}
                  </th>
                  <th className="py-1 font-medium">
                    {t('activity.fingerprint')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {items.map((row) => (
                  <tr key={row.id} className="border-t align-top">
                    <td className="whitespace-nowrap py-1.5 pr-3">
                      {new Date(row.at).toLocaleString()}
                    </td>
                    <td className="py-1.5 pr-3">
                      {row.principal_name || row.principal_id}
                    </td>
                    <td className="py-1.5 pr-3">
                      {row.action}
                      {row.op_kind ? ` / ${row.op_kind}` : ''}
                    </td>
                    <td className="py-1.5 pr-3">{row.outcome}</td>
                    <td className="py-1.5 font-mono break-all">
                      {row.fingerprint}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
