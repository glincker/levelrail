import { useTranslation } from 'react-i18next'
import { STARTERS, starterPreviewHtml, type StarterId } from './index'

/** Radio group of starters, each with a scaled live thumbnail; the iframe is sandboxed with no scripts. */
export function StarterGallery({
  value,
  onChange,
  name,
  disabled,
}: Readonly<{
  value: StarterId
  onChange: (id: StarterId) => void
  name: string
  disabled?: boolean
}>) {
  const { t } = useTranslation('pagesSites')
  return (
    <fieldset className="space-y-2" disabled={disabled}>
      <legend className="sr-only">{t('starters.label')}</legend>
      <div className="flex flex-wrap gap-1.5">
        {STARTERS.map((s) => (
          <label
            key={s.id}
            className="cursor-pointer rounded-md border border-border px-2.5 py-1 text-xs font-medium has-checked:border-ring has-checked:bg-accent has-focus-visible:ring-2 has-focus-visible:ring-ring"
          >
            <input
              type="radio"
              name="pages-starter"
              value={s.id}
              checked={value === s.id}
              onChange={() => onChange(s.id)}
              className="sr-only"
            />
            {t(`starters.${s.id}.title`)}
          </label>
        ))}
      </div>
      <div className="pointer-events-none relative h-32 w-full overflow-hidden rounded-md border border-border bg-background">
        <iframe
          title={t('starters.preview', { name: t(`starters.${value}.title`) })}
          sandbox=""
          tabIndex={-1}
          srcDoc={starterPreviewHtml(value, name)}
          className="absolute top-0 left-0 h-[430px] w-[1280px] origin-top-left scale-[0.3] border-0"
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {t(`starters.${value}.hint`)}
      </p>
    </fieldset>
  )
}
