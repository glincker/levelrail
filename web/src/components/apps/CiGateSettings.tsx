import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  PlusGlassIcon,
  ShieldCheckGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useGitSource } from '../../queries/gitSources'
import { useSetGitDeploySettings } from '../../queries/gitDeploySettings'

// CiGateSettings holds a push deploy until the commit's CI checks pass. It
// renders only once a git source is connected.
export function CiGateSettings({ appName }: { appName: string }) {
  const source = useGitSource(appName)
  if (!source.data) {
    return null
  }
  const wait = source.data.wait_for_ci ?? false
  const checks = source.data.ci_required_checks ?? []
  return (
    <CiGateForm
      key={JSON.stringify({ wait, checks })}
      appName={appName}
      wait={wait}
      checks={checks}
    />
  )
}

function CiGateForm({
  appName,
  wait,
  checks,
}: {
  appName: string
  wait: boolean
  checks: string[]
}) {
  const { t } = useTranslation('gitBuild', { keyPrefix: 'ciGate' })
  const save = useSetGitDeploySettings(appName)
  const [enabled, setEnabled] = useState(wait)
  const [names, setNames] = useState(checks)
  const [draft, setDraft] = useState('')

  const dirty =
    enabled !== wait || JSON.stringify(names) !== JSON.stringify(checks)

  const add = () => {
    const name = draft.trim()
    if (name !== '' && !names.includes(name)) {
      setNames([...names, name])
    }
    setDraft('')
  }

  const submit = () =>
    save.mutate(
      { wait_for_ci: enabled, ci_required_checks: names },
      {
        onSuccess: () => toast.add({ title: t('saved'), type: 'success' }),
        onError: (e) =>
          toast.add({
            title: t('saveFailed'),
            description: e.message,
            type: 'error',
          }),
      },
    )

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheckGlassIcon className="size-4.5 text-muted-foreground" />
          {t('title')}
        </CardTitle>
        <CardDescription>{t('description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-2">
          <Switch
            id="ci-gate-wait"
            checked={enabled}
            onCheckedChange={setEnabled}
            aria-label={t('toggle')}
          />
          <Label htmlFor="ci-gate-wait">{t('toggle')}</Label>
        </div>
        {enabled && (
          <div className="space-y-2">
            <Label htmlFor="ci-gate-check">{t('checksLabel')}</Label>
            <div className="flex flex-wrap items-center gap-1.5">
              {names.map((name) => (
                <Badge key={name} variant="outline" className="font-mono">
                  {name}
                  <button
                    type="button"
                    onClick={() => setNames(names.filter((n) => n !== name))}
                    aria-label={t('remove', { name })}
                    className="rounded-sm hover:text-foreground"
                  >
                    <XGlassIcon size="1.2em" />
                  </button>
                </Badge>
              ))}
            </div>
            <div className="flex max-w-sm items-center gap-2">
              <Input
                id="ci-gate-check"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    add()
                  }
                }}
                placeholder={t('checkPlaceholder')}
                spellCheck={false}
              />
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={add}
                disabled={draft.trim() === ''}
              >
                <PlusGlassIcon size="1.2em" />
                {t('add')}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">{t('checksHelp')}</p>
          </div>
        )}
        <Button size="sm" onClick={submit} disabled={!dirty || save.isPending}>
          {save.isPending ? t('saving') : t('save')}
        </Button>
      </CardContent>
    </Card>
  )
}
