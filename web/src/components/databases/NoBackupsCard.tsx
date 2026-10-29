import { useTranslation } from 'react-i18next'
import { CloudGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// Shown instead of the backups section for an engine that keeps nothing on
// disk, so the page never offers a backup that would capture no data.
export function NoBackupsCard({ engine }: { engine: string }) {
  const { t } = useTranslation('databases')
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CloudGlassIcon className="size-5" aria-hidden="true" />
          {t('noBackups.title')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        <p className="text-sm text-foreground">
          {t('noBackups.body', { engine })}
        </p>
        <p className="text-sm text-muted-foreground">{t('noBackups.hint')}</p>
      </CardContent>
    </Card>
  )
}
