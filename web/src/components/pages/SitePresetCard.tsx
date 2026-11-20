import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import {
  usePagesPresets,
  useSetSitePreset,
  useSitePreset,
} from '../../queries/pagesPresets'

/** Compact picker for the cache and security header preset of a site. */
export default function SitePresetCard({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const presets = usePagesPresets()
  const choice = useSitePreset(name)
  const set = useSetSitePreset(name)
  const current = choice.data?.preset ?? ''
  const active = presets.data?.find((p) => p.id === current)
  const error = set.error ?? presets.error ?? choice.error

  return (
    <section
      aria-labelledby="pages-preset-title"
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2
          id="pages-preset-title"
          className="text-sm font-semibold"
          title={t('preset.tip')}
        >
          {t('preset.title')}
        </h2>
        {presets.isSuccess && choice.isSuccess ? (
          <select
            aria-labelledby="pages-preset-title"
            className="h-8 rounded-md border border-border bg-background px-2 text-sm"
            value={current}
            disabled={set.isPending}
            onChange={(e) => set.mutate(e.target.value)}
          >
            <option value="">{t('preset.none')}</option>
            {presets.data.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        ) : null}
      </div>
      {presets.isPending || choice.isPending ? (
        <Skeleton className="h-6 w-full" />
      ) : null}
      {active ? (
        <p className="text-xs text-muted-foreground">{active.description}</p>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : null}
    </section>
  )
}
