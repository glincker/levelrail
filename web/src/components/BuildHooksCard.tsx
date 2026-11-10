import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CopyGlassIcon,
  LinkGlassIcon,
  RefreshCwGlassIcon,
  ShieldGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { InfoTip } from '@/components/kit'
import { toast } from '@/components/ui/toast'
import {
  useBuildHooks,
  useCreateBuildHook,
  useRevokeBuildHook,
  useRotateBuildHook,
  useUpdateBuildHookAllowlist,
  splitCidrs,
  type BuildHook,
} from '../queries/buildHooks'

/** BuildHooksCard manages an app's secret incoming build URLs; a URL is visible only right after create or rotate. */
export function BuildHooksCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const hooks = useBuildHooks(appName)
  const create = useCreateBuildHook(appName)
  const rotate = useRotateBuildHook(appName)
  const revoke = useRevokeBuildHook(appName)
  const updateAllow = useUpdateBuildHookAllowlist(appName)
  const [name, setName] = useState('')
  const [branch, setBranch] = useState('')
  const [allow, setAllow] = useState('')
  const [editing, setEditing] = useState('')
  const [editAllow, setEditAllow] = useState('')
  const [editAllowAny, setEditAllowAny] = useState(false)
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
    const cidrs = splitCidrs(allow)
    create.mutate(
      {
        name: name.trim(),
        branch: branch.trim(),
        ...(cidrs.length > 0 ? { allowed_cidrs: cidrs } : {}),
      },
      {
        onSuccess: (hook) => {
          setRevealed(hook)
          setName('')
          setBranch('')
          setAllow('')
        },
        onError: fail,
      },
    )
  }

  function startEdit(hook: BuildHook) {
    setEditing(hook.id)
    setEditAllow((hook.allowed_cidrs ?? []).join(', '))
    setEditAllowAny(false)
  }

  function saveAllow(e: React.FormEvent, id: string) {
    e.preventDefault()
    const cidrs = splitCidrs(editAllow)
    if (!editAllowAny && cidrs.length === 0) return
    updateAllow.mutate(
      editAllowAny ? { id, allow_any: true } : { id, allowed_cidrs: cidrs },
      {
        onSuccess: () => {
          setEditing('')
          toast.add({
            title: t('buildHooks.toast.allowSaved'),
            type: 'success',
          })
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
          <LinkGlassIcon className="size-5 text-muted-foreground" />
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
                <CopyGlassIcon className="size-5" />
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
                  <p className="text-xs text-muted-foreground">
                    {hook.allowed_cidrs && hook.allowed_cidrs.length > 0
                      ? t('buildHooks.allowList', {
                          cidrs: hook.allowed_cidrs.join(', '),
                        })
                      : t('buildHooks.allowAny')}
                    {hook.last_used_at
                      ? ` ${t('buildHooks.lastUsed', {
                          when: new Date(hook.last_used_at).toLocaleString(),
                        })}`
                      : ''}
                  </p>
                  {hook.last_refusal ? (
                    <p className="text-xs text-tone-warning">
                      {t('buildHooks.lastRefusal', {
                        reason: hook.last_refusal,
                      })}
                    </p>
                  ) : null}
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    aria-expanded={editing === hook.id}
                    onClick={() =>
                      editing === hook.id ? setEditing('') : startEdit(hook)
                    }
                  >
                    <ShieldGlassIcon className="size-5" />
                    {t('buildHooks.editAllow')}
                  </Button>
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
                    <RefreshCwGlassIcon className="size-5" />
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
                    <TrashGlassIcon className="size-5" />
                    {confirming === hook.id
                      ? t('buildHooks.confirmRevoke')
                      : t('buildHooks.revoke')}
                  </Button>
                </div>
                {editing === hook.id ? (
                  <form
                    onSubmit={(e) => saveAllow(e, hook.id)}
                    className="flex w-full flex-wrap items-end gap-2"
                  >
                    <label className="min-w-64 flex-1 space-y-1 text-xs text-muted-foreground">
                      {t('buildHooks.allowLabel')}
                      <Input
                        value={editAllow}
                        onChange={(e) => setEditAllow(e.target.value)}
                        placeholder={t('buildHooks.allowPlaceholder')}
                        disabled={editAllowAny}
                      />
                    </label>
                    <label className="flex items-center gap-2 text-xs text-muted-foreground">
                      <Checkbox
                        checked={editAllowAny}
                        onCheckedChange={(checked) =>
                          setEditAllowAny(checked === true)
                        }
                      />
                      {t('buildHooks.allowAnyToggle')}
                    </label>
                    <Button
                      type="submit"
                      size="sm"
                      disabled={
                        updateAllow.isPending ||
                        (!editAllowAny && splitCidrs(editAllow).length === 0)
                      }
                    >
                      {t('buildHooks.saveAllow')}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => setEditing('')}
                    >
                      {t('buildHooks.cancel')}
                    </Button>
                  </form>
                ) : null}
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
          <label className="space-y-1 text-xs text-muted-foreground">
            {t('buildHooks.allowLabel')}
            <Input
              value={allow}
              onChange={(e) => setAllow(e.target.value)}
              placeholder={t('buildHooks.allowPlaceholder')}
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
