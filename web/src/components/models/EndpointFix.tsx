import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RotateCwGlassIcon, WrenchGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type {
  EndpointApiType,
  EndpointDiagnosis,
} from '../../types/modelEndpoints'

const PRIVATE_ENV = 'APP_MODEL_ENDPOINT_ALLOW_PRIVATE_NETWORKS'
const OLLAMA_BIND_FIXES = new Set(['check_port', 'check_network'])

// A typed fix for a failed probe: what is wrong, what to change elsewhere,
// and a re-check button. A rejected credential can be replaced right here.
export function EndpointFix({
  diagnosis,
  apiType,
  onRecheck,
  rechecking,
  onSaveCredential,
  savingCredential,
}: {
  diagnosis: EndpointDiagnosis
  apiType: EndpointApiType
  onRecheck: () => void
  rechecking: boolean
  onSaveCredential?: (credential: string) => void
  savingCredential?: boolean
}) {
  const { t } = useTranslation('models')
  const [credential, setCredential] = useState('')
  const fix = diagnosis.fix === '' ? 'check_server' : diagnosis.fix
  const showOllama = apiType === 'ollama' && OLLAMA_BIND_FIXES.has(fix)
  return (
    <Alert variant="destructive" className="space-y-1">
      <WrenchGlassIcon size="1.4em" aria-hidden="true" />
      <AlertTitle>{t(`endpoint.fix.${fix}.title`)}</AlertTitle>
      <AlertDescription className="space-y-2">
        <p>{diagnosis.message}</p>
        <p>{t(`endpoint.fix.${fix}.body`, { env: PRIVATE_ENV })}</p>
        {fix === 'enable_internal' ? (
          <code className="block rounded-md border border-border bg-muted px-2 py-1 font-mono text-xs">
            {PRIVATE_ENV}=true
          </code>
        ) : null}
        {showOllama ? (
          <>
            <p>{t('endpoint.fix.ollamaBind')}</p>
            <code className="block rounded-md border border-border bg-muted px-2 py-1 font-mono text-xs">
              OLLAMA_HOST=0.0.0.0:11434
            </code>
          </>
        ) : null}
        {fix === 'set_credential' && onSaveCredential ? (
          <div className="flex flex-wrap items-center gap-2">
            <Input
              type="password"
              autoComplete="off"
              value={credential}
              onChange={(e) => {
                setCredential(e.target.value)
              }}
              aria-label={t('endpoint.fields.credential')}
              placeholder={t('endpoint.fields.credentialPlaceholder')}
              className="w-64"
            />
            <Button
              type="button"
              size="sm"
              disabled={credential === '' || savingCredential}
              onClick={() => {
                onSaveCredential(credential)
                setCredential('')
              }}
            >
              {t('endpoint.fix.saveAndCheck')}
            </Button>
          </div>
        ) : null}
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={rechecking}
          onClick={onRecheck}
        >
          <RotateCwGlassIcon size="1.4em" aria-hidden="true" />
          {t('endpoint.recheck')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}
