import { useId, useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { NativeSelect } from '@/components/ui/native-select'
import { cn } from '@/lib/utils'
import {
  resolveSupportedLanguage,
  supportedLanguages,
} from '../../lib/languages'

export interface LanguageSelectProps {
  /** Language code, or '' for "follow the browser" when allowDefault is set. */
  value?: string
  onChange: (code: string) => void
  /** field is a labelled form control; compact is a globe plus code button for bars. */
  variant?: 'field' | 'compact'
  /** Adds a leading "Browser default" choice whose value is ''. */
  allowDefault?: boolean
  id?: string
  className?: string
  disabled?: boolean
  /** Optional slot for a flag in the compact variant; defaults to the neutral globe. */
  renderFlag?: (code: string) => ReactNode
}

/** LanguageSelect lists every supported language by its own name with its code, using a native select so keyboard and screen readers work everywhere. */
export function LanguageSelect({
  value,
  onChange,
  variant = 'field',
  allowDefault = false,
  id,
  className,
  disabled,
  renderFlag,
}: LanguageSelectProps) {
  const { t, i18n } = useTranslation('common')
  const generatedId = useId()
  const selectId = id ?? generatedId
  const options = supportedLanguages(i18n)
  const active = value ?? resolveSupportedLanguage(i18n)
  const label = useMemo(
    () => options.find((o) => o.code === active)?.name ?? active,
    [options, active],
  )

  const list = (
    <>
      {allowDefault ? (
        <option value="">{t('language.browserDefault')}</option>
      ) : null}
      {options.map((o) => (
        <option key={o.code} value={o.code} lang={o.code}>
          {o.name} ({o.code})
        </option>
      ))}
    </>
  )

  if (variant === 'compact') {
    return (
      <div
        className={cn(
          'relative inline-flex h-9 min-w-14 items-center justify-center gap-1.5 rounded-full border border-border bg-background px-2.5 text-sm text-foreground focus-within:ring-2 focus-within:ring-ring max-sm:h-10',
          disabled && 'opacity-50',
          className,
        )}
      >
        {renderFlag?.(active) ?? (
          <GlobeGlassIcon size="1.4em" aria-hidden="true" />
        )}
        <span aria-hidden="true" className="font-medium uppercase tabular-nums">
          {active === '' ? resolveSupportedLanguage(i18n) : active}
        </span>
        <NativeSelect
          id={selectId}
          aria-label={t('language.change', { language: label })}
          title={t('language.change', { language: label })}
          value={active}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          className="absolute inset-0 h-full w-full cursor-pointer appearance-none rounded-full border-0 bg-transparent opacity-0 focus-visible:ring-0"
        >
          {list}
        </NativeSelect>
      </div>
    )
  }

  return (
    <NativeSelect
      id={selectId}
      value={active}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
      className={cn('w-full', className)}
    >
      {list}
    </NativeSelect>
  )
}
