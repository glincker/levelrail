import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import {
  useRecentlyDeleted,
  useRestoreRecentlyDeleted,
} from '../../queries/recentlyDeleted'

// Hidden while nothing was deleted inside the undo window.
export function RecentlyDeletedCard() {
  const { t } = useTranslation('lifecycle')
  const { data } = useRecentlyDeleted()
  const restore = useRestoreRecentlyDeleted()
  if (!data || data.length === 0) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('recentlyDeleted.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('recentlyDeleted.description')}
        </p>
      </CardHeader>
      <CardContent>
        <ul
          aria-label={t('recentlyDeleted.tableLabel')}
          className="divide-y divide-border"
        >
          {data.map((entry) => (
            <li
              key={`${entry.kind}/${entry.name}`}
              className="flex flex-wrap items-center justify-between gap-2 py-2"
            >
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-foreground">
                  {entry.name}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t(`recentlyDeleted.kind.${entry.kind}`)}
                  {' · '}
                  {t('recentlyDeleted.expires', {
                    date: new Date(entry.expires_at).toLocaleDateString(),
                  })}
                  {entry.last_backup
                    ? ` · ${t('recentlyDeleted.withBackup')}`
                    : ''}
                </p>
              </div>
              <Button
                size="sm"
                variant="outline"
                disabled={restore.isPending}
                onClick={() => {
                  restore.mutate(entry, {
                    onSuccess: () => {
                      toast.add({
                        title: t('recentlyDeleted.restored', {
                          name: entry.name,
                        }),
                        type: 'success',
                      })
                    },
                    onError: (err) => {
                      toast.add({
                        title: t('recentlyDeleted.restoreFailed', {
                          name: entry.name,
                          message: err.message,
                        }),
                        type: 'error',
                      })
                    },
                  })
                }}
              >
                {restore.isPending
                  ? t('recentlyDeleted.restoring')
                  : t('recentlyDeleted.restore')}
              </Button>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
