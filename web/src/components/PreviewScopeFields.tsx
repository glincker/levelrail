import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useSetPreviewPolicy } from '../queries/previewPolicy'
import type { PreviewPolicy } from '../types/previewEnvironment'

function parseServices(raw: string): string[] {
  return raw
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '')
}

// Which pull requests get a preview (label gate) and which services of a multi-service app it leaves out.
export function PreviewScopeFields({
  appName,
  policy,
}: {
  appName: string
  policy: PreviewPolicy
}) {
  const { t } = useTranslation('previews')
  const setPolicy = useSetPreviewPolicy(appName)
  const [label, setLabel] = useState(policy.require_label)
  const [services, setServices] = useState(policy.exclude_services.join(', '))
  const servicesDirty =
    parseServices(services).join(',') !== policy.exclude_services.join(',')

  function save(update: Parameters<typeof setPolicy.mutate>[0]) {
    setPolicy.mutate(update, {
      onSuccess: () => toast.add({ title: t('scope.saved'), type: 'success' }),
      onError: (error) =>
        toast.add({
          title: t('scope.error'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <label htmlFor="preview-require-label" className="text-sm font-medium">
          {t('scope.label')}
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            id="preview-require-label"
            className="min-w-0 flex-1"
            value={label}
            maxLength={100}
            placeholder={t('scope.labelPlaceholder')}
            onChange={(e) => {
              setLabel(e.target.value)
            }}
          />
          <Button
            size="sm"
            disabled={
              label.trim() === policy.require_label || setPolicy.isPending
            }
            onClick={() => {
              save({ require_label: label.trim() })
            }}
          >
            {t('scope.save')}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">{t('scope.labelHint')}</p>
      </div>

      <div className="space-y-1">
        <label
          htmlFor="preview-exclude-services"
          className="text-sm font-medium"
        >
          {t('scope.services')}
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            id="preview-exclude-services"
            className="min-w-0 flex-1 font-mono"
            value={services}
            placeholder={t('scope.servicesPlaceholder')}
            onChange={(e) => {
              setServices(e.target.value)
            }}
          />
          <Button
            size="sm"
            disabled={!servicesDirty || setPolicy.isPending}
            onClick={() => {
              save({ exclude_services: parseServices(services) })
            }}
          >
            {t('scope.save')}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          {t('scope.servicesHint')}
        </p>
      </div>
    </div>
  )
}
