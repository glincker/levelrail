import { useTranslation } from 'react-i18next'
import { GitDiffIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { useAuthEngineStatus } from '../queries/authEngine'

const MAX_MISMATCH_ROWS = 5

function Counter({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-lg border border-input p-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-lg font-medium tabular-nums">{value}</dd>
    </div>
  )
}

export function AuthEngineCard() {
  const { t } = useTranslation('settings')
  const { data: status, isLoading } = useAuthEngineStatus()

  if (isLoading || !status) return null
  const shown = status.mismatches.slice(0, MAX_MISMATCH_ROWS)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GitDiffIcon aria-hidden="true" />
          {t('authEngine.title')}
          <Badge variant="outline">{t(`authEngine.mode.${status.mode}`)}</Badge>
        </CardTitle>
        <CardDescription>
          {t(`authEngine.description.${status.mode}`)}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {t('authEngine.libraryVersion', { version: status.library_version })}
          {status.areas.length > 0
            ? ` ${t('authEngine.areas', { areas: status.areas.join(', ') })}`
            : ''}
        </p>
        {status.mode === 'shadow' ? (
          <>
            <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <Counter
                label={t('authEngine.compared')}
                value={status.compared}
              />
              <Counter label={t('authEngine.matched')} value={status.matched} />
              <Counter
                label={t('authEngine.mismatched')}
                value={status.mismatched}
              />
              <Counter label={t('authEngine.dropped')} value={status.dropped} />
            </dl>
            {shown.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('authEngine.noMismatches')}
              </p>
            ) : (
              <ul className="space-y-1 text-sm">
                {shown.map((m) => (
                  <li key={`${m.at}-${m.token_id ?? ''}-${m.kind}`}>
                    <Badge variant="outline">
                      {t(`authEngine.kind.${m.kind}`, { defaultValue: m.kind })}
                    </Badge>{' '}
                    <span className="font-mono text-xs">
                      {m.token_id ?? t('authEngine.unknownToken')}
                    </span>{' '}
                    <span className="text-muted-foreground">
                      {new Date(m.at).toLocaleString()}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
