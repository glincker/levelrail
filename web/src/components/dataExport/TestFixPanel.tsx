import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type {
  DataExportDestination,
  DataExportFix,
  UpdateDataExportDestinationRequest,
} from '../../types/dataExport'

const FIX_KEYS = {
  bad_token: 'fix.bad_token',
  tls_error: 'fix.tls_error',
  unreachable: 'fix.unreachable',
  rejected: 'fix.rejected',
  private_blocked: 'fix.private_blocked',
} as const satisfies Record<DataExportFix, string>

const TLS_KINDS = ['audit_splunk', 'audit_syslog', 'audit_otlp']
const TOKEN_KINDS = ['audit_splunk']

/** TestFixPanel turns a failed destination test into the one action that fixes it. */
export function TestFixPanel({
  destination,
  fix,
  message,
  saving,
  onApply,
  onRetest,
}: {
  destination: DataExportDestination
  fix: DataExportFix
  message: string
  saving: boolean
  onApply: (body: UpdateDataExportDestinationRequest) => void
  onRetest: () => void
}) {
  const { t } = useTranslation('dataExport')
  const [token, setToken] = useState('')
  const canTrust = fix === 'tls_error' && TLS_KINDS.includes(destination.kind)
  const canReplaceToken =
    fix === 'bad_token' && TOKEN_KINDS.includes(destination.kind)

  return (
    <Alert variant="destructive">
      <CircleAlertGlassIcon
        className="text-tone-danger"
        size="1.2em"
        aria-hidden="true"
      />
      <AlertTitle>{t('fix.title')}</AlertTitle>
      <AlertDescription className="space-y-2">
        <p className="break-words">{message}</p>
        <p>{t(FIX_KEYS[fix])}</p>
        {canTrust ? <p className="text-xs">{t('fix.trustWarning')}</p> : null}
        <div className="flex flex-wrap items-center gap-2">
          {canReplaceToken ? (
            <>
              <Input
                type="password"
                autoComplete="off"
                aria-label={t('fix.newToken')}
                placeholder={t('fix.newToken')}
                className="max-w-xs"
                value={token}
                onChange={(e) => {
                  setToken(e.target.value)
                }}
              />
              <Button
                size="sm"
                disabled={saving || token.trim() === ''}
                onClick={() => {
                  onApply({ api_key: token.trim() })
                  setToken('')
                }}
              >
                {saving ? t('fix.saving') : t('fix.saveToken')}
              </Button>
            </>
          ) : null}
          {canTrust ? (
            <Button
              size="sm"
              variant="outline"
              disabled={saving}
              onClick={() => {
                onApply({
                  options: {
                    ...destination.options,
                    insecure_skip_verify: true,
                  },
                })
              }}
            >
              {saving ? t('fix.saving') : t('fix.trustCert')}
            </Button>
          ) : null}
          {!canTrust && !canReplaceToken ? (
            <Button size="sm" variant="outline" onClick={onRetest}>
              {t('fix.retest')}
            </Button>
          ) : null}
        </div>
      </AlertDescription>
    </Alert>
  )
}
