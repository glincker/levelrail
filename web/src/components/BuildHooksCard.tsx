import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwiseIcon,
  CopyIcon,
  LinkIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { InfoTip } from '@/components/kit'
import { toast } from '@/components/ui/toast'
import {
  useBuildHooks,
  useCreateBuildHook,
  useRevokeBuildHook,
  useRotateBuildHook,
  type BuildHook,
} from '../queries/buildHooks'

/** BuildHooksCard manages an app's secret incoming build URLs; a URL is visible only right after create or rotate. */
export function BuildHooksCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const hooks = useBuildHooks(appName)
  const create = useCreateBuildHook(appName)
  const rotate = useRotateBuildHook(appName)
  const revoke = useRevokeBuildHook(appName)
  const [name, setName] = useState('')
  const [branch, setBranch] = useState('')
  const [revealed, setRevealed] = useState<BuildHook | null>(null)
  const [confirming, setConfirming] = useState('')

  function fail(error: Error) {
    toast.add({
      title: t('buildHooks.toast.errorTitle'),
      description: error.message,
      type: 'error',
    })
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    create.mutate(
      { name: name.trim(), branch: branch.trim() },
      {
        onSuccess: (hook) => {
          setRevealed(hook)
          setName('')
          setBranch('')
        },
        onError: fail,
      },
    )
  }

  function copy(url: string) {
    void navigator.clipboard.writeText(url).then(
      () => toast.add({ title: t('buildHooks.toast.copied'), type: 'success' }),
      () =>
        toast.add({ title: t('buildHooks.toast.copyFailed'), type: 'error' }),
    )
  }

  const revealedUrl = revealed?.path
    ? `${window.location.origin}${revealed.path}`
    : ''

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LinkIcon className="size-4 text-muted-foreground" />
          {t('buildHooks.title')}
          <InfoTip label={t('buildHooks.infoTipLabel')}>
            {t('buildHooks.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {revealed && revealedUrl ? (
          <div className="space-y-1 rounded-md border border-tone-warning-border bg-tone-warning-soft px-3 py-2">
            <p className="text-xs font-medium text-tone-warning">
              {t('buildHooks.shownOnce', { name: revealed.name })}
            </p>
            <div className="flex items-center gap-2">
              <Input
                readOnly
                value={revealedUrl}
                aria-label={t('buildHooks.urlLabel')}
                onFocus={(e) => e.currentTarget.select()}
              />
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => copy(revealedUrl)}
              >
                <CopyIcon className="size-4" />
                {t('buildHooks.copy')}
              </Button>
            </div>
          </div>
        ) : null}
        {hooks.data.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('buildHooks.empty')}
          </p>
        ) : (
          <ul className="divide-y divide-border">
            {hooks.data.map((hook) => (
              <li
                key={hook.id}
                className="flex flex-wrap items-center justify-between gap-3 py-2"
              >
                <div className="min-w-0 space-y-0.5">
                  <p className="text-sm font-medium text-foreground">
                    {hook.name}
                    <span className="ml-2 text-xs font-normal text-muted-foreground">
                      {hook.branch || t('buildHooks.defaultBranch')}
                    </span>
                  </p>
                  <p className="text-xs text-muted-foreground" role="status">
                    {hook.last_triggered_at
                      ? t('buildHooks.lastTriggered', {
                          when: new Date(
                            hook.last_triggered_at,
                          ).toLocaleString(),
                          result: hook.last_result ?? '',
                          count: hook.trigger_count,
                        })
                      : t('buildHooks.neverTriggered')}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={rotate.isPending}
                    onClick={() =>
                      rotate.mutate(hook.id, {
                        onSuccess: setRevealed,
                        onError: fail,
                      })
                    }
                  >
                    <ArrowsClockwiseIcon className="size-4" />
                    {t('buildHooks.rotate')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={revoke.isPending}
                    onClick={() => {
                      if (confirming !== hook.id) {
                        setConfirming(hook.id)
                        return
                      }
                      revoke.mutate(hook.id, {
                        onSuccess: () => {
                          setConfirming('')
                          if (revealed?.id === hook.id) setRevealed(null)
                        },
                        onError: fail,
                      })
                    }}
                  >
                    <TrashIcon className="size-4" />
                    {confirming === hook.id
                      ? t('buildHooks.confirmRevoke')
                      : t('buildHooks.revoke')}
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
        <form
          onSubmit={submit}
          className="flex flex-wrap items-end gap-2 border-t border-border pt-3"
        >
          <label className="space-y-1 text-xs text-muted-foreground">
            {t('buildHooks.nameLabel')}
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('buildHooks.namePlaceholder')}
              maxLength={63}
              required
            />
          </label>
          <label className="space-y-1 text-xs text-muted-foreground">
            {t('buildHooks.branchLabel')}
            <Input
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
              placeholder={t('buildHooks.branchPlaceholder')}
            />
          </label>
          <Button
            type="submit"
            size="sm"
            disabled={create.isPending || !name.trim()}
          >
            {t('buildHooks.create')}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
