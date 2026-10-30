import { useContext, useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, SaveGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { CARD_FLOAT } from './emailStyles'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import {
  useEmailBranding,
  usePreviewEmailBranding,
  useUpdateEmailBranding,
  type EmailBranding,
} from '../../queries/emailBranding'
import { BrandContext } from '../../lib/brandContext'
import { httpsStatusQueryOptions } from '../../queries/httpsStatus'
import {
  BRANDING_TEMPLATE_IDS,
  buildBrandingTemplate,
  HEX_COLOR_RE,
  type BrandingTemplateId,
} from './emailBrandingTemplates'
import { publicDomainOf } from './useEmailSuggestions'

const PREVIEW_DELAY_MS = 400

const EMPTY: EmailBranding = {
  logo_url: '',
  brand_color: '',
  footer_text: '',
  footer_address: '',
  footer_links: [],
  show_powered_by: false,
}

/** EmailBrandingCard edits the logo, accent color and footer shared by every rich email, with a scriptless live preview. */
export function EmailBrandingCard() {
  const { t } = useTranslation('settings')
  const { data: saved } = useEmailBranding()
  const [edits, setEdits] = useState<EmailBranding | null>(null)
  const [html, setHtml] = useState('')
  const save = useUpdateEmailBranding()
  const { mutate: preview, error: previewError } = usePreviewEmailBranding()
  const loaded = saved !== undefined
  const draft = edits ?? saved ?? EMPTY

  useEffect(() => {
    if (!loaded) return
    const timer = setTimeout(
      () => preview(draft, { onSuccess: (p) => setHtml(p.html) }),
      PREVIEW_DELAY_MS,
    )
    return () => clearTimeout(timer)
  }, [draft, loaded, preview])

  const maxLinks = saved?.max_footer_links ?? 5
  const brand = useContext(BrandContext)
  const https = useQuery(httpsStatusQueryOptions())
  const domain = publicDomainOf(https.data?.public_host)
  const dashboardUrl =
    domain && https.data?.state === 'issued' ? `https://${domain}` : undefined
  const isHttps = (v: string) => /^https:\/\/\S+$/i.test(v.trim())
  const logoInvalid = draft.logo_url.trim() !== '' && !isHttps(draft.logo_url)
  const colorInvalid =
    draft.brand_color.trim() !== '' && !HEX_COLOR_RE.test(draft.brand_color)
  const linkInvalid = draft.footer_links.some(
    (l) => l.url.trim() !== '' && !isHttps(l.url),
  )
  const invalid = logoInvalid || colorInvalid || linkInvalid
  const update = (patch: Partial<EmailBranding>) =>
    setEdits({ ...draft, ...patch })
  const applyTemplate = (id: BrandingTemplateId) => {
    update(
      buildBrandingTemplate(id, {
        copy: {
          text: t(`emailBranding.templates.${id}.text`),
          address:
            id === 'compliance'
              ? t('emailBranding.templates.compliance.address')
              : undefined,
          dashboardLink: t('emailBranding.templates.dashboardLink'),
        },
        variables: saved?.variables,
        currentColor: draft.brand_color,
        brandColor: brand?.PrimaryColor,
        dashboardUrl,
      }),
    )
  }

  return (
    <Card size="sm" className={CARD_FLOAT}>
      <CardContent className="space-y-3">
        <div>
          <FieldTitle>{t('emailBranding.title')}</FieldTitle>
          <FieldDescription>{t('emailBranding.description')}</FieldDescription>
        </div>
        <div className="grid gap-6 lg:grid-cols-2">
          <div className="space-y-4">
            <Field>
              <FieldLabel htmlFor="branding-logo">
                {t('emailBranding.logo')}
              </FieldLabel>
              <Input
                id="branding-logo"
                type="url"
                inputMode="url"
                value={draft.logo_url}
                onChange={(e) => update({ logo_url: e.target.value })}
                placeholder="https://"
              />
              {logoInvalid ? (
                <FieldError>{t('emailBranding.invalidHttps')}</FieldError>
              ) : (
                <FieldDescription>
                  {t('emailBranding.logoHint')}
                </FieldDescription>
              )}
            </Field>
            <Field>
              <FieldLabel htmlFor="branding-color">
                {t('emailBranding.color')}
              </FieldLabel>
              <Input
                id="branding-color"
                value={draft.brand_color}
                onChange={(e) => update({ brand_color: e.target.value })}
                placeholder={saved?.default_brand_color ?? '#107292'}
                className="sm:max-w-40"
              />
              {colorInvalid ? (
                <FieldError>{t('emailBranding.invalidColor')}</FieldError>
              ) : (
                <FieldDescription>
                  {t('emailBranding.colorHint')}
                </FieldDescription>
              )}
            </Field>
            <div
              role="group"
              aria-label={t('emailBranding.templates.label')}
              className="flex flex-wrap items-center gap-1.5"
            >
              <span className="text-xs text-muted-foreground">
                {t('emailBranding.templates.label')}
              </span>
              {BRANDING_TEMPLATE_IDS.map((id) => (
                <Button
                  key={id}
                  type="button"
                  size="xs"
                  variant="outline"
                  className="rounded-full active:translate-y-px motion-reduce:active:translate-y-0"
                  onClick={() => {
                    applyTemplate(id)
                  }}
                >
                  {t(`emailBranding.templates.${id}.name`)}
                </Button>
              ))}
            </div>
            <Field>
              <FieldLabel htmlFor="branding-footer">
                {t('emailBranding.footerText')}
              </FieldLabel>
              <Textarea
                id="branding-footer"
                rows={3}
                value={draft.footer_text}
                onChange={(e) => update({ footer_text: e.target.value })}
              />
              <FieldDescription>
                {t('emailBranding.footerHint', {
                  variables: (saved?.variables ?? [])
                    .map((v) => `{{${v}}}`)
                    .join(', '),
                })}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="branding-address">
                {t('emailBranding.address')}
              </FieldLabel>
              <Textarea
                id="branding-address"
                rows={2}
                value={draft.footer_address}
                onChange={(e) => update({ footer_address: e.target.value })}
              />
            </Field>
            <Field>
              <FieldLabel>{t('emailBranding.links')}</FieldLabel>
              <div className="space-y-2">
                {draft.footer_links.map((link, i) => (
                  <div key={i} className="flex flex-col gap-2 sm:flex-row">
                    <Input
                      aria-label={t('emailBranding.linkLabel')}
                      placeholder={t('emailBranding.linkLabel')}
                      value={link.label}
                      className="sm:max-w-40"
                      onChange={(e) =>
                        update({
                          footer_links: draft.footer_links.map((l, j) =>
                            j === i ? { ...l, label: e.target.value } : l,
                          ),
                        })
                      }
                    />
                    <Input
                      aria-label={t('emailBranding.linkUrl')}
                      aria-invalid={
                        link.url.trim() !== '' && !isHttps(link.url)
                          ? true
                          : undefined
                      }
                      placeholder="https://"
                      value={link.url}
                      onChange={(e) =>
                        update({
                          footer_links: draft.footer_links.map((l, j) =>
                            j === i ? { ...l, url: e.target.value } : l,
                          ),
                        })
                      }
                    />
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      aria-label={t('emailBranding.removeLink')}
                      onClick={() =>
                        update({
                          footer_links: draft.footer_links.filter(
                            (_, j) => j !== i,
                          ),
                        })
                      }
                    >
                      <TrashGlassIcon className="size-5" aria-hidden="true" />
                    </Button>
                  </div>
                ))}
                {linkInvalid ? (
                  <FieldError>{t('emailBranding.invalidHttps')}</FieldError>
                ) : null}
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={draft.footer_links.length >= maxLinks}
                  onClick={() =>
                    update({
                      footer_links: [
                        ...draft.footer_links,
                        { label: '', url: '' },
                      ],
                    })
                  }
                >
                  <PlusGlassIcon className="size-5" aria-hidden="true" />
                  {t('emailBranding.addLink')}
                </Button>
              </div>
            </Field>
            <Field orientation="horizontal">
              <Switch
                id="branding-powered"
                checked={draft.show_powered_by}
                onCheckedChange={(v) => update({ show_powered_by: v })}
              />
              <FieldLabel htmlFor="branding-powered">
                {t('emailBranding.poweredBy')}
              </FieldLabel>
            </Field>
            {save.isError ? (
              <Alert variant="destructive">
                <AlertDescription>{save.error.message}</AlertDescription>
              </Alert>
            ) : null}
            <Button
              type="button"
              size="sm"
              disabled={save.isPending || invalid}
              onClick={() =>
                save.mutate(draft, {
                  onSuccess: () =>
                    toast.add({
                      title: t('emailBranding.saved'),
                      type: 'success',
                    }),
                })
              }
            >
              <SaveGlassIcon className="size-5" aria-hidden="true" />
              {save.isPending
                ? t('emailBranding.saving')
                : t('emailBranding.save')}
            </Button>
          </div>
          <div className="space-y-2">
            <FieldLabel>{t('emailBranding.preview')}</FieldLabel>
            {previewError ? (
              <Alert variant="destructive">
                <AlertDescription>{previewError.message}</AlertDescription>
              </Alert>
            ) : null}
            <iframe
              title={t('emailBranding.previewTitle')}
              srcDoc={html}
              sandbox=""
              referrerPolicy="no-referrer"
              className="h-[32rem] w-full rounded-md border border-border bg-background"
            />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
