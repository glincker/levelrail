import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { StatusBrandingFields } from './StatusBrandingFields'
import { brandingValid, type BrandingValues } from '../lib/statusPageHelpers'
import { StatusPublicLinks } from './StatusPublicLinks'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import {
  useSaveStatusPageSettings,
  useStatusPageSettings,
  useStatusPreview,
  type StatusPageSettings,
} from '../queries/statusPage'

const STATUS_VARIANT = {
  operational: 'success',
  degraded: 'warning',
  outage: 'destructive',
  maintenance: 'outline',
  unknown: 'muted',
} as const

function SettingsForm({ initial }: { initial: StatusPageSettings }) {
  const { t } = useTranslation('statusPage')
  const [enabled, setEnabled] = useState(initial.enabled)
  const [title, setTitle] = useState(initial.title)
  const [description, setDescription] = useState(initial.description)
  const [domain, setDomain] = useState(initial.custom_domain)
  const [branding, setBranding] = useState<BrandingValues>({
    accent: initial.accent ?? '',
    logo_url: initial.logo_url ?? '',
    about: initial.about ?? '',
    website: initial.website ?? '',
    noindex: initial.noindex ?? false,
    powered_by: initial.powered_by ?? true,
  })
  const save = useSaveStatusPageSettings()

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(
          {
            enabled,
            title,
            description,
            custom_domain: domain,
            ...branding,
            accent: branding.accent.trim(),
            logo_url: branding.logo_url.trim(),
            about: branding.about.trim(),
            website: branding.website.trim(),
          },
          {
            onSuccess: () => {
              toast.add({ title: t('settings.saved'), type: 'success' })
            },
          },
        )
      }}
    >
      <label
        className="flex items-center gap-3 text-sm font-medium"
        htmlFor="sp-enabled"
      >
        <Switch
          id="sp-enabled"
          checked={enabled}
          onCheckedChange={setEnabled}
        />
        {t('settings.publish')}
      </label>
      <Field>
        <FieldLabel htmlFor="sp-title">{t('settings.title')}</FieldLabel>
        <Input
          id="sp-title"
          value={title}
          placeholder={t('settings.titlePlaceholder')}
          onChange={(e) => {
            setTitle(e.target.value)
          }}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor="sp-desc">{t('settings.description')}</FieldLabel>
        <Input
          id="sp-desc"
          value={description}
          onChange={(e) => {
            setDescription(e.target.value)
          }}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor="sp-domain">
          {t('settings.customDomain')}
        </FieldLabel>
        <Input
          id="sp-domain"
          value={domain}
          placeholder={t('settings.domainPlaceholder')}
          onChange={(e) => {
            setDomain(e.target.value)
          }}
        />
        <FieldDescription>{t('settings.domainHint')}</FieldDescription>
      </Field>
      <StatusBrandingFields value={branding} onChange={setBranding} />
      {save.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {save.error.message}
        </p>
      ) : null}
      <Button
        type="submit"
        disabled={save.isPending || !brandingValid(branding)}
      >
        {save.isPending ? t('settings.saving') : t('settings.save')}
      </Button>
    </form>
  )
}

function PreviewCard() {
  const { t } = useTranslation('statusPage')
  const { data, isError } = useStatusPreview()
  if (isError || !data) return null
  return (
    <section
      className="space-y-2 rounded-lg border border-border p-4"
      aria-label={t('settings.previewTitle')}
    >
      <h2 className="text-sm font-semibold text-foreground">
        {t('settings.previewTitle')}
      </h2>
      <p className="text-sm text-muted-foreground">
        {data.title}: {data.status_text}
      </p>
      {data.components.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('settings.previewEmpty')}
        </p>
      ) : (
        <ul className="divide-y divide-border text-sm">
          {data.components.map((c) => (
            <li
              key={c.name}
              className="flex items-center justify-between gap-3 py-1.5"
            >
              <span>{c.name}</span>
              <span className="flex items-center gap-2 text-xs text-muted-foreground">
                {c.uptime_90d === null
                  ? t('settings.noData')
                  : t('settings.uptime', { value: c.uptime_90d.toFixed(2) })}
                <Badge
                  variant={
                    STATUS_VARIANT[c.status as keyof typeof STATUS_VARIANT] ??
                    'muted'
                  }
                >
                  {t(`status.${c.status}`, { defaultValue: c.status })}
                </Badge>
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

export function StatusPageSettingsPanel() {
  const { t } = useTranslation('statusPage')
  const { data, isLoading, error } = useStatusPageSettings()
  if (isLoading) {
    return <ListSkeleton rows={3} />
  }
  if (error || !data) {
    return <p className="text-sm text-destructive">{error?.message}</p>
  }
  return (
    <div className="space-y-6">
      {data.enabled ? (
        <a
          href={data.public_path ?? '/public/status'}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1.5 text-sm text-primary underline underline-offset-2"
        >
          {t('settings.openPublic')}
          <ExternalLinkGlassIcon className="size-5" aria-hidden="true" />
        </a>
      ) : (
        <p className="text-sm text-muted-foreground">{t('settings.off')}</p>
      )}
      <SettingsForm key={data.title + String(data.enabled)} initial={data} />
      <StatusPublicLinks settings={data} />
      <PreviewCard />
    </div>
  )
}
