import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PasswordRotationCard } from './PasswordRotationCard'
import { RevealCredentialCard } from './RevealCredentialCard'

// The source list shows only what works today; more sources join once they do.
function CredentialsSourceCard() {
  const { t } = useTranslation('dbNav')
  return (
    <Card>
      <CardHeader className="space-y-1">
        <CardTitle className="text-sm">
          {t('credentials.sourceTitle')}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <label className="flex items-start gap-2 text-sm">
          <input
            type="radio"
            name="credentials-source"
            checked
            readOnly
            className="mt-1"
          />
          <span>
            <span className="font-medium">{t('credentials.managed')}</span>
            <span className="block text-xs text-muted-foreground">
              {t('credentials.managedHint')}
            </span>
          </span>
        </label>
      </CardContent>
    </Card>
  )
}

/** CredentialsPanel is the database Credentials page: the platform credential, its source and rotation. */
export function CredentialsPanel({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('dbNav')
  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
          {t('credentials.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('credentials.description')}
        </p>
      </div>
      <CredentialsSourceCard />
      <RevealCredentialCard databaseName={databaseName} />
      <PasswordRotationCard databaseName={databaseName} />
    </div>
  )
}
