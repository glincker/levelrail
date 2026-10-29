import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { WandGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { useLintDockerfile } from '../../queries/dockerIntegration'

/** DockerfileLintCard checks pasted Dockerfile text and applies the safe fixes in place. */
export function DockerfileLintCard() {
  const { t } = useTranslation('images')
  const [text, setText] = useState('')
  const lint = useLintDockerfile()
  const result = lint.data
  const fixable = result?.findings.some(
    (f) => f.fixable && f.rule === 'root-user',
  )

  function run(source: string) {
    lint.mutate(
      { dockerfile: source, hasDockerignore: true },
      {
        onError: (error) =>
          toast.add({
            title: t('lint.toast.failed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  function applyFix() {
    if (!result) return
    setText(result.fixed_dockerfile)
    run(result.fixed_dockerfile)
    toast.add({ title: t('lint.toast.fixed'), type: 'success' })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('lint.title')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <p className="text-xs text-muted-foreground">{t('lint.help')}</p>
        <Textarea
          value={text}
          onChange={(e) => {
            setText(e.target.value)
          }}
          rows={6}
          className="font-mono"
          aria-label={t('lint.inputLabel')}
          placeholder={t('lint.placeholder')}
        />
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            disabled={!text.trim() || lint.isPending}
            onClick={() => {
              run(text)
            }}
          >
            {t('lint.check')}
          </Button>
          {fixable ? (
            <Button size="sm" variant="outline" onClick={applyFix}>
              <WandGlassIcon size="1.2em" aria-hidden="true" />
              {t('lint.applyFix')}
            </Button>
          ) : null}
        </div>
        {result && result.findings.length === 0 ? (
          <p className="text-sm text-tone-success">{t('lint.clean')}</p>
        ) : null}
        <ul className="flex flex-col gap-2">
          {result?.findings
            .filter((f) => f.rule !== 'no-dockerignore')
            .map((f) => (
              <li key={`${f.rule}-${f.line}`} className="flex gap-2 text-sm">
                <Badge variant={f.severity === 'warn' ? 'warning' : 'info'}>
                  {t(`lint.severity.${f.severity}`)}
                </Badge>
                <span>
                  {f.line > 0 ? t('lint.line', { line: f.line }) + ' ' : ''}
                  {f.message}
                </span>
              </li>
            ))}
        </ul>
      </CardContent>
    </Card>
  )
}
