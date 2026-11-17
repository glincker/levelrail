import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  oracleGuardKind,
  type OracleGuardKind,
} from '../../lib/oracleProvision'

const SETTING: Record<OracleGuardKind, string> = {
  enabled: 'provision.oracle.enabled',
  a1: 'provision.oracle.max_a1_ocpu, provision.oracle.max_a1_memory_gb',
  micro: 'provision.oracle.max_e2_micro',
  paidShape: 'provision.oracle.allow_paid_shapes',
  unknownPrice: 'provision.oracle.allow_unknown_price',
}

// Explains an Oracle create refusal and names the setting that controls it.
// The server text stays visible because it carries the actual counts.
export function OracleGuardNotice({ message }: { message: string }) {
  const { t } = useTranslation('nodes')
  const kind = oracleGuardKind(message)
  return (
    <Alert variant="destructive">
      <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
      <AlertDescription>
        <span className="block font-medium">{t('oracle.guard.title')}</span>
        {kind ? (
          <span className="block">
            {t(`oracle.guard.${kind}`, { setting: SETTING[kind] })}
          </span>
        ) : null}
        <span className="block text-muted-foreground">{message}</span>
      </AlertDescription>
    </Alert>
  )
}
