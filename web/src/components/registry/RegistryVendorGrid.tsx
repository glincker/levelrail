import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { BrandTile } from '../BrandLogoBadge'
import { UrlDialog } from '../kit/UrlDialog'
import { useUrlDialog } from '../kit/useUrlDialog'
import { useRegistryVendors } from '../../queries/registryVendors'
import type { RegistryVendor } from '../../types/registryVendor'
import { RegistryVendorForm } from './RegistryVendorForm'

function VendorTile({ vendor }: { vendor: RegistryVendor }) {
  const { t } = useTranslation('settings')
  const triggerRef = useRef<HTMLButtonElement>(null)
  const dialog = useUrlDialog(vendor.id)
  return (
    <li className="flex items-center gap-3 rounded-lg border border-border p-3">
      <BrandTile name={vendor.brand} label={vendor.name} />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{vendor.name}</p>
        {(vendor.default_host ?? vendor.host_hint) && (
          <p className="truncate font-mono text-xs text-muted-foreground">
            {vendor.default_host ?? vendor.host_hint}
          </p>
        )}
      </div>
      <Button
        ref={triggerRef}
        type="button"
        variant="outline"
        size="sm"
        className="shrink-0"
        aria-label={t('registryVendors.connectFor', { vendor: vendor.name })}
        onClick={() => dialog.open()}
      >
        <PlugGlassIcon size="1.4em" aria-hidden="true" />
        {t('registryVendors.connect')}
      </Button>
      <UrlDialog
        value={vendor.id}
        returnFocusRef={triggerRef}
        title={t('registryVendors.dialogTitle', { vendor: vendor.name })}
        description={t('registryVendors.dialogDescription')}
      >
        <RegistryVendorForm vendor={vendor} onDone={dialog.close} />
      </UrlDialog>
    </li>
  )
}

// One tile per vendor preset. A tile opens its own deep-linkable dialog,
// so /settings/registry-credentials?connect=ghcr lands on the GHCR form.
export function RegistryVendorGrid() {
  const { t } = useTranslation('settings')
  const vendors = useRegistryVendors()
  if (vendors.isLoading || vendors.isError || !vendors.data) return null
  return (
    <section aria-labelledby="registry-vendors-heading" className="space-y-3">
      <div>
        <h2 id="registry-vendors-heading" className="text-sm font-medium">
          {t('registryVendors.heading')}
        </h2>
        <p className="text-xs text-muted-foreground">
          {t('registryVendors.subheading')}
        </p>
      </div>
      <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {vendors.data.map((v) => (
          <VendorTile key={v.id} vendor={v} />
        ))}
      </ul>
    </section>
  )
}
