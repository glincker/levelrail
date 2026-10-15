import { useTranslation } from 'react-i18next'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { DataExportKind } from '../../types/dataExport'

export interface VendorValues {
  apiKey: string
  region: string
  site: string
  dataset: string
}

/** VendorFields is the key and per-vendor options of a vendor log destination. */
export function VendorFields({
  kind,
  values,
  onChange,
}: {
  kind: DataExportKind
  values: VendorValues
  onChange: (next: Partial<VendorValues>) => void
}) {
  const { t } = useTranslation('dataExport')
  return (
    <>
      <Field>
        <FieldLabel htmlFor="export-api-key">{t('form.apiKey')}</FieldLabel>
        <Input
          id="export-api-key"
          type="password"
          autoComplete="off"
          value={values.apiKey}
          onChange={(e) => {
            onChange({ apiKey: e.target.value })
          }}
        />
        <FieldDescription>{t('form.apiKeyHint')}</FieldDescription>
      </Field>
      {kind === 'logs_newrelic' || kind === 'logs_axiom' ? (
        <Field>
          <FieldLabel htmlFor="export-region">{t('form.region')}</FieldLabel>
          <Select
            value={values.region}
            onValueChange={(v) => {
              if (typeof v === 'string') onChange({ region: v })
            }}
          >
            <SelectTrigger id="export-region" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="us">{t('form.regionUs')}</SelectItem>
              <SelectItem value="eu">{t('form.regionEu')}</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      ) : null}
      {kind === 'logs_datadog' ? (
        <Field>
          <FieldLabel htmlFor="export-site">{t('form.site')}</FieldLabel>
          <Input
            id="export-site"
            value={values.site}
            placeholder="datadoghq.com"
            onChange={(e) => {
              onChange({ site: e.target.value })
            }}
          />
        </Field>
      ) : null}
      {kind === 'logs_axiom' ? (
        <Field>
          <FieldLabel htmlFor="export-dataset">{t('form.dataset')}</FieldLabel>
          <Input
            id="export-dataset"
            value={values.dataset}
            onChange={(e) => {
              onChange({ dataset: e.target.value })
            }}
          />
        </Field>
      ) : null}
    </>
  )
}
