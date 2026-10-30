import { useTranslation } from 'react-i18next'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  STATUS_LIMITS,
  isValidAccent,
  isValidLogoUrl,
  isValidWebsite,
  type BrandingValues,
} from '../lib/statusPageHelpers'

const DEFAULT_ACCENT = '#3b5bdb'

export function StatusBrandingFields({
  value,
  onChange,
}: {
  value: BrandingValues
  onChange: (next: BrandingValues) => void
}) {
  const { t } = useTranslation('statusPage')
  const set = <K extends keyof BrandingValues>(k: K, v: BrandingValues[K]) => {
    onChange({ ...value, [k]: v })
  }
  const accentOk = isValidAccent(value.accent)
  const logoOk = isValidLogoUrl(value.logo_url)
  const websiteOk = isValidWebsite(value.website)
  const aboutLen = value.about.length
  const swatch = /^#[0-9a-fA-F]{6}$/.test(value.accent.trim())
    ? value.accent.trim()
    : DEFAULT_ACCENT

  return (
    <fieldset className="space-y-3">
      <legend className="text-sm font-semibold text-foreground">
        {t('branding.title')}
      </legend>
      <div className="grid gap-3 sm:grid-cols-3">
        <Field>
          <FieldLabel htmlFor="sp-accent">{t('branding.accent')}</FieldLabel>
          <div className="flex items-center gap-2">
            <input
              type="color"
              value={swatch}
              aria-label={t('branding.accentPicker')}
              className="h-8 w-9 shrink-0 cursor-pointer rounded-md border border-border bg-transparent p-0.5"
              onChange={(e) => {
                set('accent', e.target.value)
              }}
            />
            <Input
              id="sp-accent"
              value={value.accent}
              placeholder={DEFAULT_ACCENT}
              spellCheck={false}
              aria-invalid={!accentOk}
              aria-describedby={accentOk ? undefined : 'sp-accent-err'}
              className="font-mono"
              onChange={(e) => {
                set('accent', e.target.value)
              }}
            />
          </div>
          {accentOk ? null : (
            <p
              id="sp-accent-err"
              role="alert"
              className="text-xs text-destructive"
            >
              {t('branding.accentError')}
            </p>
          )}
        </Field>
        <Field className="sm:col-span-2">
          <FieldLabel htmlFor="sp-logo">{t('branding.logo')}</FieldLabel>
          <Input
            id="sp-logo"
            type="url"
            value={value.logo_url}
            placeholder="https://example.com/logo.svg"
            aria-invalid={!logoOk}
            aria-describedby="sp-logo-hint"
            onChange={(e) => {
              set('logo_url', e.target.value)
            }}
          />
          {logoOk ? (
            <FieldDescription id="sp-logo-hint">
              {t('branding.logoHint')}
            </FieldDescription>
          ) : (
            <p
              id="sp-logo-hint"
              role="alert"
              className="text-xs text-destructive"
            >
              {t('branding.logoError')}
            </p>
          )}
        </Field>
      </div>
      <Field>
        <FieldLabel htmlFor="sp-website">{t('branding.website')}</FieldLabel>
        <Input
          id="sp-website"
          type="url"
          value={value.website}
          placeholder="https://example.com"
          aria-invalid={!websiteOk}
          aria-describedby={websiteOk ? undefined : 'sp-website-err'}
          onChange={(e) => {
            set('website', e.target.value)
          }}
        />
        {websiteOk ? null : (
          <p
            id="sp-website-err"
            role="alert"
            className="text-xs text-destructive"
          >
            {t('branding.websiteError')}
          </p>
        )}
      </Field>
      <Field>
        <div className="flex items-baseline justify-between gap-2">
          <FieldLabel htmlFor="sp-about">{t('branding.about')}</FieldLabel>
          <span
            className={
              aboutLen > STATUS_LIMITS.about
                ? 'text-xs text-destructive'
                : 'text-xs text-muted-foreground'
            }
          >
            {t('branding.aboutCount', {
              count: aboutLen,
              max: STATUS_LIMITS.about,
            })}
          </span>
        </div>
        <Textarea
          id="sp-about"
          rows={3}
          value={value.about}
          aria-invalid={aboutLen > STATUS_LIMITS.about}
          onChange={(e) => {
            set('about', e.target.value)
          }}
        />
        <FieldDescription>{t('branding.aboutHint')}</FieldDescription>
      </Field>
      <div className="grid gap-3 sm:grid-cols-2">
        <label
          className="flex items-start gap-3 text-sm font-medium"
          htmlFor="sp-noindex"
        >
          <Switch
            id="sp-noindex"
            checked={value.noindex}
            onCheckedChange={(v) => {
              set('noindex', v)
            }}
          />
          <span>
            {t('branding.noindex')}
            <span className="block text-xs font-normal text-muted-foreground">
              {t('branding.noindexHint')}
            </span>
          </span>
        </label>
        <label
          className="flex items-start gap-3 text-sm font-medium"
          htmlFor="sp-powered"
        >
          <Switch
            id="sp-powered"
            checked={value.powered_by}
            onCheckedChange={(v) => {
              set('powered_by', v)
            }}
          />
          <span>{t('branding.poweredBy')}</span>
        </label>
      </div>
    </fieldset>
  )
}
