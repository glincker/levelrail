import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { useAuthEngineStatus } from '../queries/authEngine'

function Feature({ label, on }: { label: string; on: boolean }) {
  const { t } = useTranslation('settings')
  return (
    <div className="flex items-center justify-between rounded-lg border border-input p-3">
      <dt className="text-sm">{label}</dt>
      <dd>
        <Badge variant={on ? 'default' : 'outline'}>
          {on ? t('authEngine.available') : t('authEngine.unavailable')}
        </Badge>
      </dd>
    </div>
  )
}

export function AuthEngineCard() {
  const { t } = useTranslation('settings')
  const { data: status, isLoading } = useAuthEngineStatus()

  if (isLoading || !status) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheckIcon aria-hidden="true" />
          {t('authEngine.title')}
        </CardTitle>
        <CardDescription>
          {t('authEngine.description', { version: status.library_version })}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <dl className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <Feature label={t('authEngine.totp')} on={status.totp} />
          <Feature label={t('authEngine.passkeys')} on={status.passkeys} />
          <Feature label={t('authEngine.oauth')} on={status.oauth} />
        </dl>
      </CardContent>
    </Card>
  )
}
