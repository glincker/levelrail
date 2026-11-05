import { useTranslation } from 'react-i18next'
import { SearchGlassIcon, CircleXGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

export function RuntimeSettingsToolbar({
  query,
  onQuery,
  changedOnly,
  onChangedOnly,
  shown,
  total,
}: {
  query: string
  onQuery: (next: string) => void
  changedOnly: boolean
  onChangedOnly: (next: boolean) => void
  shown: number
  total: number
}) {
  const { t } = useTranslation('runtimeSettings')
  return (
    <div
      role="search"
      className="rounded-lg border border-border bg-background px-3 py-2"
    >
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-48 flex-1">
          <SearchGlassIcon
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            value={query}
            aria-label={t('toolbar.searchLabel')}
            placeholder={t('toolbar.searchPlaceholder')}
            className="pl-8"
            onChange={(e) => {
              onQuery(e.target.value)
            }}
          />
        </div>
        <Button
          type="button"
          size="sm"
          variant={changedOnly ? 'default' : 'outline'}
          aria-pressed={changedOnly}
          onClick={() => {
            onChangedOnly(!changedOnly)
          }}
        >
          {t('toolbar.changedOnly')}
        </Button>
        {query !== '' || changedOnly ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => {
              onQuery('')
              onChangedOnly(false)
            }}
          >
            <CircleXGlassIcon size="1.4em" aria-hidden="true" />
            {t('toolbar.clear')}
          </Button>
        ) : null}
        <span role="status" className="text-xs text-muted-foreground">
          {t('toolbar.count', { shown, total })}
        </span>
      </div>
    </div>
  )
}
