import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BanGlassIcon,
  ShieldCheckGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useDeleteFirewallRule, type FwRule } from '../../queries/firewallEdge'
import { VirtualRows } from './VirtualRows'
import type { FirewallActions } from './useFirewallActions'

/** IpListCard is a first-class view over the rule model: single-IP rules with one action, with an add form and removal. */
export function IpListCard({
  kind,
  rules,
  scope,
  actions,
}: {
  kind: 'block' | 'bypass'
  rules: FwRule[]
  scope: string
  actions: FirewallActions
}) {
  const { t } = useTranslation('firewall')
  const del = useDeleteFirewallRule()
  const [ip, setIp] = useState('')
  const [note, setNote] = useState('')
  const [error, setError] = useState<string | null>(null)
  const block = kind === 'block'

  function add(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const value = ip.trim()
    if (!value) {
      setError(t('ipList.required'))
      return
    }
    const done = () => {
      setIp('')
      setNote('')
    }
    if (block) {
      actions.blockIp(value, {
        note: note.trim(),
        onDone: done,
        onError: setError,
      })
    } else {
      actions.allowIp(value, {
        note: note.trim(),
        onDone: done,
        onError: setError,
      })
    }
  }

  function remove(rule: FwRule) {
    del.mutate(rule.id, {
      onSuccess: () =>
        toast.add({
          title: t('ipList.removed', {
            ip: rule.conditions[0]?.value ?? rule.name,
          }),
          type: 'success',
        }),
      onError: (err) =>
        toast.add({
          title: t('actions.failed'),
          description: err.message,
          type: 'error',
        }),
    })
  }

  return (
    <Card className="min-w-0">
      <CardHeader className="border-b">
        <CardTitle>
          {block ? t('ipList.blockTitle') : t('ipList.bypassTitle')}
        </CardTitle>
        <CardDescription>
          {block ? t('ipList.blockHelp') : t('ipList.bypassHelp')}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <form
          onSubmit={add}
          className="flex flex-wrap items-start gap-2"
          noValidate
        >
          <div className="min-w-40 flex-1 basis-56 space-y-1">
            <Input
              aria-label={t('ipList.ipLabel')}
              aria-invalid={error ? true : undefined}
              className="font-mono"
              placeholder={t('placeholder.ip')}
              value={ip}
              onChange={(e) => setIp(e.target.value)}
            />
            {error ? (
              <p role="alert" className="text-xs text-destructive">
                {error}
              </p>
            ) : null}
          </div>
          <Input
            aria-label={t('ipList.noteLabel')}
            className="min-w-40 flex-1 basis-56"
            placeholder={t('ipList.notePlaceholder')}
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
          <Button type="submit">
            {block ? t('ipList.block') : t('ipList.bypass')}
          </Button>
        </form>
        {rules.length === 0 ? (
          <EmptyState
            compact
            icon={
              block ? (
                <BanGlassIcon className="size-5.5" />
              ) : (
                <ShieldCheckGlassIcon className="size-5.5 text-tone-success" />
              )
            }
            title={
              block ? t('ipList.blockEmptyTitle') : t('ipList.bypassEmptyTitle')
            }
            description={
              block ? t('ipList.blockEmptyHelp') : t('ipList.bypassEmptyHelp')
            }
          />
        ) : (
          <VirtualRows
            items={rules}
            rowHeight={44}
            label={block ? t('ipList.blockTitle') : t('ipList.bypassTitle')}
            getKey={(r) => r.id}
            renderRow={(r) => (
              <div className="flex items-center gap-3 px-2 py-2 text-sm">
                <span
                  className="min-w-0 flex-1 truncate font-mono text-[13px]"
                  title={r.conditions[0]?.value}
                >
                  {r.conditions[0]?.value}
                </span>
                {r.scope ? (
                  <span
                    className="max-w-40 truncate font-mono text-xs text-muted-foreground"
                    title={r.scope}
                  >
                    {r.scope}
                  </span>
                ) : null}
                <span
                  className="hidden min-w-0 flex-1 truncate text-xs text-muted-foreground sm:block"
                  title={r.note}
                >
                  {r.note}
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={del.isPending}
                  aria-label={t('ipList.removeFor', {
                    ip: r.conditions[0]?.value ?? r.name,
                  })}
                  onClick={() => remove(r)}
                >
                  <TrashGlassIcon size="1.2em" />
                </Button>
              </div>
            )}
          />
        )}
        <p className="text-xs text-muted-foreground">
          {scope ? t('ipList.scopeNote', { scope }) : t('ipList.allHostsNote')}
        </p>
      </CardContent>
    </Card>
  )
}
