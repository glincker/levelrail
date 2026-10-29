import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PencilGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import {
  useSaveMailLayout,
  type MailLayout,
  type MailTemplate,
} from '../../queries/mail'

function LayoutCard({ layout }: { layout: MailLayout }) {
  const { t } = useTranslation('mail')
  const [image, setImage] = useState(layout.header_image_url)
  const [footer, setFooter] = useState(layout.footer_text)
  const [error, setError] = useState<string | null>(null)
  const save = useSaveMailLayout()
  const dirty =
    image !== layout.header_image_url || footer !== layout.footer_text

  return (
    <Card>
      <CardContent className="space-y-4">
        <div>
          <h2 className="text-sm font-medium">{t('templates.layout.title')}</h2>
          <p className="text-sm text-muted-foreground">
            {t('templates.layout.description')}
          </p>
        </div>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="mail-layout-image">
              {t('templates.layout.headerImage')}
            </FieldLabel>
            <Input
              id="mail-layout-image"
              type="url"
              inputMode="url"
              value={image}
              placeholder="https://"
              onChange={(e) => setImage(e.target.value)}
            />
            <FieldDescription>
              {t('templates.layout.headerImageHelp')}
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="mail-layout-footer">
              {t('templates.layout.footer')}
            </FieldLabel>
            <Textarea
              id="mail-layout-footer"
              rows={3}
              value={footer}
              onChange={(e) => setFooter(e.target.value)}
            />
            <FieldDescription>
              {t('templates.layout.footerHelp', {
                examples: '{{brand_name}}, {{year}}',
              })}
            </FieldDescription>
          </Field>
        </FieldGroup>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <Button
          type="button"
          size="sm"
          disabled={!dirty || save.isPending}
          writeAction
          onClick={() => {
            setError(null)
            save.mutate(
              { header_image_url: image.trim(), footer_text: footer },
              {
                onSuccess: () =>
                  toast.add({
                    title: t('templates.layout.saved'),
                    type: 'success',
                  }),
                onError: (e) => setError(e.message),
              },
            )
          }}
        >
          {t('templates.layout.save')}
        </Button>
      </CardContent>
    </Card>
  )
}

/** TemplateList shows every email template with its state, plus the shared layout settings. */
export function TemplateList({
  templates,
  layout,
  onEdit,
}: {
  templates: MailTemplate[]
  layout: MailLayout
  onEdit: (key: string) => void
}) {
  const { t } = useTranslation('mail')
  return (
    <div className="space-y-6">
      <section aria-label={t('templates.listHeading')}>
        <ul className="divide-y divide-border rounded-lg border border-border">
          {templates.map((tpl) => (
            <li
              key={tpl.key}
              className="flex flex-wrap items-center justify-between gap-3 px-4 py-3"
            >
              <div className="min-w-0 space-y-0.5">
                <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                  {tpl.label}
                  <Badge
                    variant={tpl.category === 'announcement' ? 'info' : 'muted'}
                  >
                    {t(`templates.kind.${tpl.category}`)}
                  </Badge>
                  <Badge variant={tpl.customized ? 'accent' : 'outline'}>
                    {tpl.customized
                      ? t('templates.customized')
                      : t('templates.builtIn')}
                  </Badge>
                </p>
                <p className="text-sm text-muted-foreground">
                  {tpl.description}
                </p>
                <p className="truncate font-mono text-xs text-muted-foreground">
                  {tpl.current.subject}
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => onEdit(tpl.key)}
              >
                <PencilGlassIcon size="1.4em" data-icon="inline-start" />
                {t('templates.edit')}
              </Button>
            </li>
          ))}
        </ul>
      </section>
      <LayoutCard
        layout={layout}
        key={`${layout.header_image_url}|${layout.footer_text}`}
      />
    </div>
  )
}
