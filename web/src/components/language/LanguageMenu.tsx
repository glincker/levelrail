import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  ChevronDownGlassIcon,
  GlobeGlassIcon,
} from '@thesvg/ui-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { normalizeDetectedLanguage } from '../../i18n/detect'
import { htmlLang } from '../../i18n/direction'
import { languageFlag } from '../../lib/languageFlags'
import { isSupportedLanguage, supportedLanguages } from '../../lib/languages'

export interface LanguageMenuProps {
  value: string
  onChange: (code: string) => void
  disabled?: boolean
  className?: string
}

// Flags are decorative (the text label always carries the meaning), so the image has empty alt.
function Flag({ code, className }: { code: string; className?: string }) {
  const entry = languageFlag(code)
  if (!entry) return <GlobeGlassIcon size="1.4em" aria-hidden="true" />
  return (
    <img
      src={entry.flag}
      alt=""
      aria-hidden="true"
      draggable={false}
      className={cn(
        'size-5 shrink-0 rounded-full ring-1 ring-border ring-inset',
        className,
      )}
    />
  )
}

/** LanguageMenu is the flag dropdown for bars: native names first, English names muted, current language checked. */
export function LanguageMenu({
  value,
  onChange,
  disabled,
  className,
}: LanguageMenuProps) {
  const { t, i18n } = useTranslation('common')
  const options = supportedLanguages(i18n)
  const nameOf = (code: string) =>
    languageFlag(code)?.native ??
    options.find((o) => o.code === code)?.name ??
    code
  const browser = normalizeDetectedLanguage(
    typeof navigator === 'undefined' ? '' : navigator.language,
  )
  const suggestion =
    browser !== value && isSupportedLanguage(i18n, browser) ? browser : ''
  const label = t('language.change', { language: nameOf(value) })

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        disabled={disabled}
        aria-label={label}
        title={label}
        className={cn(
          'inline-flex h-9 min-w-14 cursor-pointer items-center justify-center gap-1.5 rounded-full border border-border bg-background px-2.5 text-sm text-foreground outline-hidden hover:bg-foreground/5 focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-50 max-sm:h-10',
          className,
        )}
      >
        <Flag code={value} />
        <span aria-hidden="true" className="font-medium uppercase tabular-nums">
          {value}
        </span>
        <ChevronDownGlassIcon
          size="1em"
          aria-hidden="true"
          className="text-muted-foreground"
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        className="max-h-80 w-64 overflow-y-auto p-1.5"
      >
        <div className="px-2 pt-1 pb-1.5 text-xs font-medium text-muted-foreground">
          {t('language.menuTitle')}
        </div>
        {suggestion ? (
          <>
            <DropdownMenuItem
              className="min-h-11 gap-2.5"
              lang={suggestion}
              onClick={() => onChange(suggestion)}
            >
              <Flag code={suggestion} />
              <span className="flex min-w-0 flex-1 flex-col leading-tight">
                <span className="truncate font-medium">
                  {t('language.useBrowser')}
                </span>
                <span className="truncate text-xs text-muted-foreground">
                  {nameOf(suggestion)}
                </span>
              </span>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        ) : null}
        {options.map((o) => {
          const entry = languageFlag(o.code)
          const current = o.code === value
          return (
            <DropdownMenuItem
              key={o.code}
              lang={htmlLang(o.code)}
              aria-current={current ? 'true' : undefined}
              className={cn(
                'min-h-11 gap-2.5',
                current && 'bg-foreground/5 text-foreground',
              )}
              onClick={() => onChange(o.code)}
            >
              <Flag code={o.code} />
              <span className="flex min-w-0 flex-1 flex-col leading-tight">
                <span className="truncate font-medium">
                  {entry?.native ?? o.name}
                </span>
                <span className="truncate text-xs text-muted-foreground">
                  {entry?.english ?? o.code}
                </span>
              </span>
              {current ? (
                <CheckGlassIcon
                  size="1.2em"
                  aria-hidden="true"
                  className="text-primary"
                />
              ) : null}
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
