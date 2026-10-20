import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useVolumeGuide } from '../queries/migration'

export function MigrationVolumesCard() {
  const { t } = useTranslation('migration')
  const [source, setSource] = useState('')
  const guide = useVolumeGuide()

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    guide.mutate({ source: source.trim() })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('volumes.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {t('volumes.description')}
        </p>
        <form onSubmit={onSubmit} className="flex flex-wrap items-end gap-2">
          <div className="min-w-64 flex-1 space-y-1.5">
            <Label htmlFor="volume-source">{t('volumes.source')}</Label>
            <Input
              id="volume-source"
              required
              placeholder={t('volumes.sourcePlaceholder')}
              value={source}
              onChange={(e) => setSource(e.target.value)}
            />
          </div>
          <Button type="submit" disabled={guide.isPending}>
            {guide.isPending ? t('volumes.loading') : t('volumes.show')}
          </Button>
        </form>
        {guide.isError ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t('volumes.loadError')}: {guide.error.message}
            </AlertDescription>
          </Alert>
        ) : null}
        {guide.data && guide.data.guides.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('volumes.none')}</p>
        ) : null}
        {guide.data && guide.data.guides.length > 0 ? (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              {guide.data.warning}
            </p>
            {guide.data.guides.map((g) => (
              <div key={`${g.app}-${g.target}-${g.container_path}`}>
                <p className="text-xs font-medium">
                  {g.app}: {g.source} to {g.container_path}
                </p>
                <pre
                  className="mt-1 overflow-x-auto rounded-md bg-muted p-2 font-mono text-xs"
                  aria-label={t('volumes.copyCommand')}
                >
                  {g.command}
                </pre>
              </div>
            ))}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
