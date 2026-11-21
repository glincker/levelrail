import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { CodeBlock } from '@/components/ui/code-block'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { AwsLinkKit } from '../../types/cloudAccounts'

export function AwsLinkKitStep({ kit }: Readonly<{ kit: AwsLinkKit }>) {
  const { t } = useTranslation('cloudAccounts')
  return (
    <div className="space-y-3">
      <Alert>
        <AlertTitle>{t('wizard.kit.readFirstTitle')}</AlertTitle>
        <AlertDescription>{t('wizard.kit.readFirst')}</AlertDescription>
      </Alert>
      <div className="flex flex-wrap items-center gap-3">
        {kit.quick_create_url ? (
          <Button
            size="sm"
            render={
              <a
                href={kit.quick_create_url}
                target="_blank"
                rel="noopener noreferrer"
              />
            }
          >
            {t('wizard.kit.quickCreate')}
          </Button>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t('wizard.kit.noQuickCreate')}
          </p>
        )}
        <p className="text-xs text-muted-foreground">
          {t('wizard.kit.externalId')}{' '}
          <code className="font-mono text-foreground">{kit.external_id}</code>
        </p>
      </div>
      <Tabs defaultValue="policy">
        <TabsList aria-label={t('wizard.kit.formats')}>
          <TabsTrigger value="policy">{t('wizard.kit.policy')}</TabsTrigger>
          <TabsTrigger value="cloudformation">
            {t('wizard.kit.cloudformation')}
          </TabsTrigger>
          <TabsTrigger value="terraform">
            {t('wizard.kit.terraform')}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="policy">
          <CodeBlock
            code={kit.policy_json}
            language="json"
            label={t('wizard.kit.policy')}
            maxHeight="lg"
          />
        </TabsContent>
        <TabsContent value="cloudformation">
          <CodeBlock
            code={kit.template_yaml}
            language="yaml"
            label={t('wizard.kit.cloudformation')}
            maxHeight="lg"
          />
        </TabsContent>
        <TabsContent value="terraform">
          <CodeBlock
            code={kit.terraform}
            language="hcl"
            label={t('wizard.kit.terraform')}
            maxHeight="lg"
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}
