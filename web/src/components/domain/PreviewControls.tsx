import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { NativeSelect, PreviewPanel } from './PolicyShared'
import type { PreviewResult } from '../../queries/domainPolicyTypes'

const METHODS = ['GET', 'HEAD', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS']

// PreviewControls lets the operator pick a sample request and shows how the
// current draft would treat it.
export function PreviewControls({
  id,
  method,
  path,
  onMethod,
  onPath,
  data,
  loading,
}: {
  id: string
  method: string
  path: string
  onMethod: (m: string) => void
  onPath: (p: string) => void
  data?: PreviewResult
  loading: boolean
}) {
  const { t } = useTranslation('domainPolicies')
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-end gap-2">
        <NativeSelect
          id={`${id}-method`}
          label={t('preview.method')}
          value={method}
          options={METHODS.map((m) => ({ value: m, label: m }))}
          onChange={onMethod}
        />
        <label className="flex flex-1 flex-col gap-1 text-xs">
          <span className="text-muted-foreground">{t('preview.path')}</span>
          <Input
            value={path}
            onChange={(e) => onPath(e.target.value)}
            placeholder="/api/users"
          />
        </label>
      </div>
      <PreviewPanel
        steps={data?.steps}
        loading={loading}
        errors={data?.errors}
      />
    </div>
  )
}
