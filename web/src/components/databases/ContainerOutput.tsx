import { useTranslation } from 'react-i18next'
import type { ProvisionAttempt } from '../../queries/databaseProvision'

type Failure = NonNullable<ProvisionAttempt['failure']>

// Classified crash reason plus the collapsed last output of the database container.
export function ContainerOutput({ failure }: { failure: Failure }) {
  const { t } = useTranslation('databases')
  const { cause, log_tail: tail } = failure
  if (!cause && !tail) return null
  return (
    <div className="space-y-2" data-testid="container-output">
      {cause ? (
        <div className="rounded-md bg-muted p-2 text-sm">
          <p className="font-medium text-foreground">
            {t(`provision.cause.${cause.kind}.title` as 'provision.title')}
          </p>
          <p className="text-muted-foreground">
            {t(`provision.cause.${cause.kind}.hint` as 'provision.title', {
              defaultValue: cause.hint,
            })}
          </p>
        </div>
      ) : null}
      {tail ? (
        <details className="text-xs">
          <summary className="cursor-pointer text-muted-foreground">
            {t('provision.containerOutput')}
          </summary>
          <pre className="mt-1 max-h-64 overflow-auto rounded-md bg-muted p-2 font-mono whitespace-pre-wrap break-words">
            {tail}
          </pre>
        </details>
      ) : null}
    </div>
  )
}
