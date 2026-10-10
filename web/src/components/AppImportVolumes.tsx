import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  useAppImportVolumes,
  useSetAppImportVolume,
  type AppImportSession,
} from '../queries/appImport'

export function AppImportVolumes({
  session,
  onBack,
  onNext,
}: {
  session: AppImportSession
  onBack: () => void
  onNext: () => void
}) {
  const { t } = useTranslation('migration')
  const [source, setSource] = useState('')
  const guide = useAppImportVolumes()
  const mark = useSetAppImportVolume()
  const withVolumes = session.items.filter(
    (i) => i.selected && (i.volumes?.length ?? 0) > 0 && i.target,
  )
  const pending = withVolumes.reduce(
    (n, i) => n + (i.volumes ?? []).filter((v) => !v.copied).length,
    0,
  )

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    guide.mutate({ id: session.id, source: source.trim() })
  }

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">
          {t('appImport.volumes.title')}
        </h3>
        <p className="text-sm text-muted-foreground">
          {t('appImport.volumes.description')}
        </p>
      </div>
      {withVolumes.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('appImport.volumes.none')}
        </p>
      ) : (
        <>
          <form onSubmit={onSubmit} className="flex flex-wrap items-end gap-2">
            <div className="min-w-64 flex-1 space-y-1.5">
              <Label htmlFor="appimport-vol-source">
                {t('appImport.volumes.source')}
              </Label>
              <Input
                id="appimport-vol-source"
                required
                placeholder={t('appImport.volumes.sourcePlaceholder')}
                value={source}
                onChange={(e) => setSource(e.target.value)}
              />
            </div>
            <Button type="submit" disabled={guide.isPending}>
              {t('appImport.volumes.show')}
            </Button>
          </form>
          {guide.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{guide.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {guide.data ? (
            <p className="text-sm text-muted-foreground">
              {guide.data.warning}
            </p>
          ) : null}
          <ul className="space-y-3">
            {withVolumes.map((it) => (
              <li
                key={it.source_id}
                className="space-y-2 rounded-md border p-3"
              >
                <p className="text-sm font-medium">{it.name}</p>
                {(it.volumes ?? []).map((v, idx) => {
                  const g = guide.data?.guides.find(
                    (x) => x.source_id === it.source_id && x.index === idx,
                  )
                  return (
                    <div key={`${v.name}-${idx}`} className="space-y-1">
                      <Label className="flex items-center gap-2 text-sm font-normal">
                        <Checkbox
                          aria-label={t('appImport.volumes.confirm', {
                            path: v.container_path,
                          })}
                          checked={v.copied}
                          disabled={mark.isPending}
                          onCheckedChange={(c) =>
                            mark.mutate({
                              id: session.id,
                              item: it.source_id,
                              index: idx,
                              copied: c === true,
                            })
                          }
                        />
                        {t('appImport.volumes.confirm', {
                          path: v.container_path,
                        })}
                      </Label>
                      {g ? (
                        <pre
                          className="overflow-x-auto rounded-md bg-muted p-2 font-mono text-xs"
                          aria-label={t('appImport.volumes.command')}
                        >
                          {g.command}
                        </pre>
                      ) : null}
                    </div>
                  )
                })}
              </li>
            ))}
          </ul>
        </>
      )}
      <div className="flex items-center gap-2">
        <Button variant="outline" onClick={onBack}>
          {t('appImport.back')}
        </Button>
        <Button onClick={onNext}>{t('appImport.volumes.continue')}</Button>
        {pending > 0 ? (
          <span className="text-xs text-muted-foreground">
            {t('appImport.volumes.pending', { count: pending })}
          </span>
        ) : null}
      </div>
    </div>
  )
}
