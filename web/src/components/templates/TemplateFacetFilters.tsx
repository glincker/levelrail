import { useTranslation } from 'react-i18next'
import { NativeSelect } from '@/components/ui/native-select'
import type { TemplateFacets } from '../../lib/templateCatalogView'
import type { LicenseClass, TemplateArch } from '../../queries/serviceTemplates'

const LICENSE_CLASSES: LicenseClass[] = [
  'permissive',
  'copyleft',
  'network-copyleft',
  'source-available',
  'proprietary',
]
const ARCHES: TemplateArch[] = ['amd64', 'arm64']

/** TemplateFacetFilters is the license-class and architecture filter pair of the toolbar. */
export function TemplateFacetFilters({
  facets,
  onChange,
}: {
  facets: TemplateFacets
  onChange: (next: TemplateFacets) => void
}) {
  const { t } = useTranslation('templates')
  return (
    <>
      <NativeSelect
        aria-label={t('facts.filter.license')}
        value={facets.licenseClass ?? ''}
        onChange={(e) => {
          onChange({
            ...facets,
            licenseClass: (e.target.value || undefined) as
              LicenseClass | undefined,
          })
        }}
      >
        <option value="">{t('facts.filter.anyLicense')}</option>
        {LICENSE_CLASSES.map((c) => (
          <option key={c} value={c}>
            {t(`facts.license.classes.${c}`)}
          </option>
        ))}
      </NativeSelect>
      <NativeSelect
        aria-label={t('facts.filter.arch')}
        value={facets.arch ?? ''}
        onChange={(e) => {
          onChange({
            ...facets,
            arch: (e.target.value || undefined) as TemplateArch | undefined,
          })
        }}
      >
        <option value="">{t('facts.filter.anyArch')}</option>
        {ARCHES.map((a) => (
          <option key={a} value={a}>
            {a}
          </option>
        ))}
      </NativeSelect>
    </>
  )
}
