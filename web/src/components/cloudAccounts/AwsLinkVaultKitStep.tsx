import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { CodeBlock } from '@/components/ui/code-block'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { AwsVaultKit } from '../../types/cloudAccounts'

export function AwsLinkVaultKitStep({ kit }: Readonly<{ kit: AwsVaultKit }>) {
  const { t } = useTranslation('cloudAccounts')
  const tabs = [
    { id: 'commands', code: kit.vault_commands, language: 'bash' },
    { id: 'vaultPolicy', code: kit.vault_policy, language: 'hcl' },
    { id: 'policy', code: kit.policy_json, language: 'json' },
    { id: 'trust', code: kit.trust_policy_json, language: 'json' },
  ] as const
  return (
    <div className="space-y-3">
      <Alert>
        <AlertTitle>{t('wizard.vault.kitTitle')}</AlertTitle>
        <AlertDescription>
          {t('wizard.vault.kitHint', { arn: kit.role_arn })}
        </AlertDescription>
      </Alert>
      <Tabs defaultValue="commands">
        <TabsList aria-label={t('wizard.vault.kitFormats')}>
          {tabs.map((x) => (
            <TabsTrigger key={x.id} value={x.id}>
              {t(`wizard.vault.tabs.${x.id}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        {tabs.map((x) => (
          <TabsContent key={x.id} value={x.id}>
            <CodeBlock
              code={x.code}
              language={x.language}
              label={t(`wizard.vault.tabs.${x.id}`)}
              maxHeight="lg"
            />
          </TabsContent>
        ))}
      </Tabs>
    </div>
  )
}
