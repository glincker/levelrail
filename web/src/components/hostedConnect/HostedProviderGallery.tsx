import { useTranslation } from 'react-i18next'
import { DatabaseGlassIcon } from '@thesvg/ui-react'
import { SelectableCard } from '@/components/ui/selectable-card'
import { GENERIC_PROVIDER } from '@/lib/hostedConnect'
import { ENGINES } from '../connectDatabaseForm'
import type { ExternalProvider } from '../../types/externalDatabase'
import { ProviderMark } from './ProviderMark'

// Step 1: where the database runs. Picking a tile moves on at once.
export function HostedProviderGallery({
  providers,
  loading,
  failed,
  onPick,
}: Readonly<{
  providers: readonly ExternalProvider[]
  loading: boolean
  failed: boolean
  onPick: (id: string) => void
}>) {
  const { t } = useTranslation('databases')
  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium">
        {t('external.hosted.pickProvider')}
      </h3>
      {failed ? (
        <p className="text-sm text-destructive" role="alert">
          {t('external.hosted.providersError')}
        </p>
      ) : null}
      <ul aria-busy={loading} className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {providers
          .filter((p) => p.id !== GENERIC_PROVIDER)
          .map((p) => (
            <li key={p.id}>
              <SelectableCard
                selected={false}
                onSelect={() => {
                  onPick(p.id)
                }}
                className="flex w-full items-center gap-2.5 rounded-lg p-2.5 text-left"
              >
                <ProviderMark brand={p.brand} label={p.label} />
                <span className="min-w-0">
                  <span className="block text-sm font-medium">{p.label}</span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {p.engines.join(', ')}
                  </span>
                </span>
              </SelectableCard>
            </li>
          ))}
        <li>
          <SelectableCard
            selected={false}
            onSelect={() => {
              onPick(GENERIC_PROVIDER)
            }}
            className="flex w-full items-center gap-2.5 rounded-lg p-2.5 text-left"
          >
            <DatabaseGlassIcon
              className="size-6 text-muted-foreground"
              aria-hidden="true"
            />
            <span className="min-w-0">
              <span className="block text-sm font-medium">
                {t('external.hosted.providerOther')}
              </span>
              <span className="block truncate text-xs text-muted-foreground">
                {ENGINES.join(', ')}
              </span>
            </span>
          </SelectableCard>
        </li>
      </ul>
    </div>
  )
}
