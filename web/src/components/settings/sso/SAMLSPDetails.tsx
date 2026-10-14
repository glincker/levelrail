import { useTranslation } from 'react-i18next'
import type { SAMLConnection } from '@/queries/sso'
import { CopyField } from '@/components/traffic/CopyField'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { FormSection } from '@/components/ui/form-section'

/** SAMLSPDetails shows the service provider values an IdP admin copies. */
export function SAMLSPDetails({ connection }: { connection?: SAMLConnection }) {
  const { t } = useTranslation('sso')
  const rows = connection?.sp_acs_url
    ? [
        { label: t('form.sp.entityId'), value: connection.sp_entity_id },
        { label: t('form.sp.acsUrl'), value: connection.sp_acs_url },
        { label: t('form.sp.metadataUrl'), value: connection.sp_metadata_url },
      ]
    : []
  return (
    <FormSection
      className="lg:grid-cols-1"
      title={t('form.sp.title')}
      description={t('form.sp.description')}
    >
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('form.sp.saveFirst')}
        </p>
      ) : (
        <FieldGroup className="gap-3">
          {rows.map((r) => (
            <Field key={r.label}>
              <FieldLabel>{r.label}</FieldLabel>
              <CopyField
                value={r.value}
                label={t('form.sp.copy', { label: r.label })}
              />
            </Field>
          ))}
        </FieldGroup>
      )}
    </FormSection>
  )
}
