import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  PlusGlassIcon,
  ShieldCheckGlassIcon,
  TriangleAlertGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
  // The API answers 422 when no CI reader can be built for this source.
  const [unreadable, setUnreadable] = useState(false)

  const dirty =
    enabled !== wait || JSON.stringify(names) !== JSON.stringify(checks)

  const add = () => {
    const name = draft.trim()
    if (name !== '' && !names.includes(name)) {
      setNames([...names, name])
    }
    setDraft('')
  }

  const submit = () => {
    setUnreadable(false)
    save.mutate(
      { wait_for_ci: enabled, ci_required_checks: names },
      {
        onSuccess: () => toast.add({ title: t('saved'), type: 'success' }),
        onError: (e) => {
          setUnreadable(e.status === 422)
          toast.add({
            title: t('saveFailed'),
            description: e.status === 422 ? undefined : e.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheckGlassIcon className="size-5 text-muted-foreground" />
          {t('title')}
        </CardTitle>
        <CardDescription>{t('description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-2">
          <Switch
            id="ci-gate-wait"
            checked={enabled}
            onCheckedChange={(next) => {
              setEnabled(next)
              setUnreadable(false)
            }}
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
                    <XGlassIcon size="1.4em" />
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
                <PlusGlassIcon size="1.4em" />
                {t('add')}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">{t('checksHelp')}</p>
            <p className="text-xs text-muted-foreground">{t('gitlabNote')}</p>
          </div>
        )}
        {unreadable && (
          <Alert variant="destructive" role="alert">
            <TriangleAlertGlassIcon size="1.7em" />
            <AlertTitle>{t('unreadable.title')}</AlertTitle>
            <AlertDescription>
              <p>{t('unreadable.intro')}</p>
              <ul className="list-disc pl-5">
                <li>{t('unreadable.ssh')}</li>
                <li>{t('unreadable.gitea')}</li>
                <li>{t('unreadable.bitbucket')}</li>
                <li>{t('unreadable.githubNoApp')}</li>
              </ul>
              <p>{t('unreadable.fix')}</p>
            </AlertDescription>
          </Alert>
        )}
        <Button size="sm" onClick={submit} disabled={!dirty || save.isPending}>
          {save.isPending ? t('saving') : t('save')}
        </Button>
      </CardContent>
    </Card>
  )
}
