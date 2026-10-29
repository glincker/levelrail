import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DownloadGlassIcon } from '@thesvg/ui-react'
import type { SAMLConnection } from '@/queries/sso'
import { useImportSAMLMetadata } from '@/queries/sso'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { FormSection } from '@/components/ui/form-section'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'

/** SAMLMetadataImport fills the IdP fields from a metadata URL or pasted XML. */
export function SAMLMetadataImport({
  connectionId,
  onImported,
}: {
  connectionId: string
  onImported: (c: SAMLConnection) => void
}) {
  const { t } = useTranslation('sso')
  const importMetadata = useImportSAMLMetadata()
  const [url, setUrl] = useState('')
  const [xml, setXml] = useState('')
  const ready = (url.trim() === '') !== (xml.trim() === '')

  const run = () => {
    importMetadata.mutate(
      {
        id: connectionId,
        url: url.trim() || undefined,
        xml: xml.trim() || undefined,
      },
      {
        onSuccess: (c) => {
          toast.add({ title: t('form.import.done'), type: 'success' })
          setXml('')
          onImported(c)
        },
      },
    )
  }

  return (
    <FormSection
      className="lg:grid-cols-1"
      title={t('form.import.title')}
      description={t('form.import.description')}
      error={importMetadata.isError ? importMetadata.error.message : undefined}
    >
      <FieldGroup className="gap-3">
        <Field>
          <FieldLabel htmlFor={`${connectionId}-md-url`}>
            {t('form.import.url')}
          </FieldLabel>
          <Input
            id={`${connectionId}-md-url`}
            type="url"
            className="font-mono"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${connectionId}-md-xml`}>
            {t('form.import.xml')}
          </FieldLabel>
          <Textarea
            id={`${connectionId}-md-xml`}
            rows={4}
            className="font-mono text-xs"
            value={xml}
            onChange={(e) => setXml(e.target.value)}
          />
        </Field>
        <div>
          <Button
            type="button"
            variant="outline"
            disabled={!ready || importMetadata.isPending}
            onClick={run}
          >
            <DownloadGlassIcon size="1.2em" aria-hidden="true" />
            {t('form.import.button')}
          </Button>
        </div>
      </FieldGroup>
    </FormSection>
  )
}
