import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { useIsRoot } from '@/hooks/useIsRoot'
import {
  SCAN_GATES,
  supplyChainDefaultQueryOptions,
  useSaveSupplyChainDefault,
  type ScanGate,
  type SupplyChainDefault,
} from '@/queries/supplyChainDefault'

function toGate(v: string): ScanGate {
  return SCAN_GATES.find((g) => g === v) ?? 'off'
}

function PolicyForm({ policy }: { policy: SupplyChainDefault }) {
  const { t } = useTranslation('security', { keyPrefix: 'imageScanning' })
  const isRoot = useIsRoot()
  const save = useSaveSupplyChainDefault()
  const [enabled, setEnabled] = useState(policy.scan_enabled)
  const [gate, setGate] = useState<ScanGate>(toGate(policy.scan_gate))
  const invalid = !enabled && gate !== 'off'
  const dirty =
    enabled !== policy.scan_enabled || gate !== toGate(policy.scan_gate)
  return (
    <form
      className="flex flex-wrap items-end gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!invalid) save.mutate({ scan_enabled: enabled, scan_gate: gate })
      }}
    >
      <div className="flex items-center gap-2">
        <Switch
          id="scan-enabled"
          checked={enabled}
          onCheckedChange={setEnabled}
          disabled={!isRoot}
        />
        <label htmlFor="scan-enabled" className="text-sm">
          {t('scan.label')}
        </label>
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor="scan-gate" className="text-sm text-muted-foreground">
          {t('gate.label')}
        </label>
        <NativeSelect
          id="scan-gate"
          value={gate}
          disabled={!isRoot}
          onChange={(e) => setGate(toGate(e.target.value))}
        >
          {SCAN_GATES.map((g) => (
            <option key={g} value={g}>
              {t(`gate.options.${g}`)}
            </option>
          ))}
        </NativeSelect>
      </div>
      {isRoot ? (
        <Button type="submit" disabled={invalid || !dirty || save.isPending}>
          {save.isPending ? t('save.pending') : t('save.action')}
        </Button>
      ) : (
        <p className="text-sm text-muted-foreground">{t('rootOnly')}</p>
      )}
      {invalid ? (
        <p role="alert" className="basis-full text-sm text-destructive">
          {t('gate.needsScan')}
        </p>
      ) : null}
      {save.isError ? (
        <p role="alert" className="basis-full text-sm text-destructive">
          {save.error.message}
        </p>
      ) : null}
      {save.isSuccess && !dirty ? (
        <p role="status" className="basis-full text-sm text-muted-foreground">
          {t('save.done')}
        </p>
      ) : null}
    </form>
  )
}

export function ImageScanningPage() {
  const { t } = useTranslation('security', { keyPrefix: 'imageScanning' })
  const policy = useQuery(supplyChainDefaultQueryOptions())
  return (
    <PageTemplate title={t('page.title')} description={t('page.description')}>
      {policy.isPending ? (
        <p className="text-sm text-muted-foreground">{t('loading')}</p>
      ) : policy.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {t('error')}
        </p>
      ) : (
        <div className="space-y-3">
          <PolicyForm policy={policy.data} />
          <p className="text-sm text-muted-foreground">{t('inherit')}</p>
          {!policy.data.server_enabled ? (
            <p className="text-sm text-muted-foreground">{t('serverOff')}</p>
          ) : null}
        </div>
      )}
    </PageTemplate>
  )
}
