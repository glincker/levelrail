import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useMailVersions,
  useRestoreMailVersion,
  type MailVersion,
} from '../../queries/mail'

/** TemplateHistory lists the last saved versions of a template and restores one. */
export function TemplateHistory({
  eventKey,
  onRestored,
}: {
  eventKey: string
  onRestored: (version: MailVersion) => void
}) {
  const { t } = useTranslation('mail')
  const { data: versions = [] } = useMailVersions(eventKey)
  const restore = useRestoreMailVersion(eventKey)

  return (
    <section aria-label={t('templates.history.title')} className="space-y-3">
      <div>
        <h3 className="text-sm font-medium">{t('templates.history.title')}</h3>
        <p className="text-xs text-muted-foreground">
          {t('templates.history.description')}
        </p>
      </div>
      {versions.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('templates.history.empty')}
        </p>
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border">
          {versions.map((v) => (
            <li
              key={v.version}
              className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
            >
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">
                  {t('templates.history.version', { version: v.version })}:{' '}
                  {v.subject}
                </p>
                <p className="text-xs text-muted-foreground">
                  {new Date(v.created_at).toLocaleString()}
                  {v.created_by
                    ? ` ${t('templates.history.by', { who: v.created_by })}`
                    : ''}
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={restore.isPending}
                writeAction
                onClick={() =>
                  restore.mutate(v.version, {
                    onSuccess: () => {
                      onRestored(v)
                      toast.add({
                        title: t('templates.history.restored'),
                        type: 'success',
                      })
                    },
                    onError: (error) =>
                      toast.add({ title: error.message, type: 'error' }),
                  })
                }
              >
                {t('templates.history.restore')}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
