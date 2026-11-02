import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../hooks/useIsRoot'
import {
  useResetRuntimeSetting,
  useRuntimeSettings,
  useSetRuntimeSetting,
} from '../queries/runtimeSettings'

export const FALLBACK_CERTS_SETTING = 'ingress.fallback_certs'

// Applies immediately (stored runtime setting), so it sits outside the
// card's Save form.
export function IngressFallbackCertsCard() {
  const { t } = useTranslation('ingressProtection')
  const isRoot = useIsRoot()
  const { data } = useRuntimeSettings()
  const save = useSetRuntimeSetting()
  const reset = useResetRuntimeSetting()
  const setting = data?.settings.find((s) => s.key === FALLBACK_CERTS_SETTING)
  if (!setting) return null
  const value = String(setting.value)
  const busy = save.isPending || reset.isPending
  const failed = (err: Error) => {
    toast.add({
      title: t('fallbackCerts.failed'),
      description: err.message,
      type: 'error',
    })
  }

  return (
    <div
      id="fallback-certs"
      className="scroll-mt-20 rounded-md border border-border p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <FieldLabel htmlFor="fallback-certs-select">
            {t('fallbackCerts.label')}
          </FieldLabel>
          <Badge
            variant={
              setting.source === 'saved'
                ? 'info'
                : setting.source === 'env'
                  ? 'accent'
                  : 'muted'
            }
          >
            {t(`fallbackCerts.source.${setting.source}`)}
          </Badge>
        </div>
        <div className="flex items-center gap-2">
          <Select
            value={value}
            disabled={!isRoot || busy}
            onValueChange={(next) => {
              if (next === null || next === value) return
              save.mutate(
                { key: setting.key, value: next },
                {
                  onSuccess: () => {
                    toast.add({
                      title: t('fallbackCerts.saved'),
                      type: 'success',
                    })
                  },
                  onError: failed,
                },
              )
            }}
          >
            <SelectTrigger
              id="fallback-certs-select"
              size="sm"
              aria-label={t('fallbackCerts.label')}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(setting.allowed_strings ?? ['eager', 'lazy']).map((word) => (
                <SelectItem key={word} value={word}>
                  {t(`fallbackCerts.options.${word}`, { defaultValue: word })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {isRoot && setting.source === 'saved' ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={() => {
                reset.mutate(
                  { key: setting.key },
                  {
                    onSuccess: () => {
                      toast.add({
                        title: t('fallbackCerts.resetDone'),
                        type: 'success',
                      })
                    },
                    onError: failed,
                  },
                )
              }}
            >
              {t('fallbackCerts.reset')}
            </Button>
          ) : null}
        </div>
      </div>
      <FieldDescription className="mt-1">
        {t('fallbackCerts.helper')}
      </FieldDescription>
      {setting.source === 'env' ? (
        <FieldDescription className="mt-1">
          {t('fallbackCerts.envHint', { env: setting.env })}
        </FieldDescription>
      ) : null}
    </div>
  )
}
