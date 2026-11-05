import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { CodeBlock } from '@/components/ui/code-block'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { InfoTip } from '@/components/kit'
import { useCITemplates } from '../queries/ciDeploy'
import { useCreateToken } from '../queries/tokens'
import type { TokenAbility } from '../types/token'

/** CITemplatesCard hands out the pipeline file for any supported CI, and a least-privilege deployer token for the ones that need one. */
export function CITemplatesCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const templates = useCITemplates(appName)
  const createToken = useCreateToken()
  const [selected, setSelected] = useState(templates.data.templates[0]?.id)
  const [revealed, setRevealed] = useState('')

  const current = templates.data.templates.find((tpl) => tpl.id === selected)
  if (!current) return null

  function makeToken() {
    createToken.mutate(
      {
        name: `ci-${appName}`,
        abilities: templates.data.deployer_abilities as TokenAbility[],
      },
      {
        onSuccess: (res) => setRevealed(res.token),
        onError: (error) =>
          toast.add({
            title: t('ciTemplates.token.errorTitle'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {t('ciTemplates.title')}
          <InfoTip label={t('ciTemplates.infoTipLabel')}>
            {t('ciTemplates.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div
          className="flex flex-wrap gap-1.5"
          role="group"
          aria-label={t('ciTemplates.providerLabel')}
        >
          {templates.data.templates.map((tpl) => (
            <Button
              key={tpl.id}
              type="button"
              size="sm"
              variant={tpl.id === current.id ? 'secondary' : 'outline'}
              aria-pressed={tpl.id === current.id}
              onClick={() => {
                setSelected(tpl.id)
                setRevealed('')
              }}
            >
              {t(`ciTemplates.providers.${tpl.id}`, { defaultValue: tpl.id })}
            </Button>
          ))}
        </div>
        <p className="text-xs text-muted-foreground">
          {t('ciTemplates.saveAs', { target: current.target })}
        </p>
        <CodeBlock
          code={current.content}
          language="yaml"
          label={current.target}
          maxHeight="md"
        />
        {templates.data.needs_domain ? (
          <p className="text-xs text-tone-warning" role="status">
            {t('ciTemplates.needsDomain')}
          </p>
        ) : null}
        {current.auth === 'oidc' ? (
          <p className="text-xs text-muted-foreground">
            {t('ciTemplates.oidcNote')}
          </p>
        ) : (
          <div className="space-y-2 border-t border-border pt-3">
            <p className="text-xs text-muted-foreground">
              {t('ciTemplates.token.help', {
                abilities: templates.data.deployer_abilities.join(', '),
              })}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={makeToken}
              disabled={createToken.isPending}
            >
              {t('ciTemplates.token.create')}
            </Button>
            {revealed ? (
              <div className="space-y-1">
                <Input
                  readOnly
                  value={revealed}
                  className="font-mono"
                  aria-label={t('ciTemplates.token.label')}
                  onFocus={(e) => e.currentTarget.select()}
                />
                <p className="text-xs text-muted-foreground">
                  {t('ciTemplates.token.shownOnce')}
                </p>
              </div>
            ) : null}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
