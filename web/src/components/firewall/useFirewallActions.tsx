import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BanGlassIcon,
  CircleCheckGlassIcon,
  ExternalLinkGlassIcon,
  PlusGlassIcon,
} from '@thesvg/ui-react'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import type { ActionMenuItem } from '@/components/kit/ActionMenu'
import { toast } from '@/components/ui/toast'
import {
  FirewallError,
  useAllowFirewallIp,
  useBlockFirewallIp,
  type FwField,
  type FwTopTable,
} from '../../queries/firewallEdge'

export interface RuleSeed {
  field: FwField
  value: string
}

export interface IpOptions {
  note?: string
  /** Runs after the entry is saved. */
  onDone?: () => void
  /** Runs with the server's message instead of the default failure toast. */
  onError?: (message: string) => void
}

export interface FirewallActions {
  /** Row menu entries for one row of a top table. */
  menuFor: (table: FwTopTable, key: string) => ActionMenuItem[]
  blockIp: (ip: string, opts?: IpOptions) => void
  allowIp: (ip: string, opts?: IpOptions) => void
  /** Render once in the view: the confirmation shown when a block would lock the caller out. */
  dialog: ReactNode
}

const TABLE_FIELD: Partial<Record<FwTopTable, FwField>> = {
  user_agent: 'user_agent',
  path: 'path',
  host: 'host',
}

/** useFirewallActions wires the row menus (block, allow, create a rule) to the real firewall calls. */
export function useFirewallActions({
  scope,
  onCreateRule,
}: {
  scope: string
  onCreateRule: (seed: RuleSeed) => void
}): FirewallActions {
  const { t } = useTranslation('firewall')
  const block = useBlockFirewallIp()
  const allow = useAllowFirewallIp()
  const [lockout, setLockout] = useState<{
    ip: string
    opts?: IpOptions
  } | null>(null)

  function fail(e: unknown, opts?: IpOptions) {
    const message = e instanceof Error ? e.message : ''
    if (opts?.onError) {
      opts.onError(message)
      return
    }
    toast.add({
      title: t('actions.failed'),
      description: message || undefined,
      type: 'error',
    })
  }

  function blockIp(ip: string, opts?: IpOptions, force = false) {
    block.mutate(
      { ip, scope: scope || undefined, note: opts?.note || undefined, force },
      {
        onSuccess: () => {
          setLockout(null)
          opts?.onDone?.()
          toast.add({ title: t('actions.blocked', { ip }), type: 'success' })
        },
        onError: (e) => {
          if (e instanceof FirewallError && e.code === 'self_lockout') {
            setLockout({ ip, opts })
            return
          }
          fail(e, opts)
        },
      },
    )
  }

  function allowIp(ip: string, opts?: IpOptions) {
    allow.mutate(
      { ip, scope: scope || undefined, note: opts?.note || undefined },
      {
        onSuccess: () => {
          opts?.onDone?.()
          toast.add({ title: t('actions.allowed', { ip }), type: 'success' })
        },
        onError: (e) => fail(e, opts),
      },
    )
  }

  function menuFor(table: FwTopTable, key: string): ActionMenuItem[] {
    if (table === 'ip' || table === 'denied_ip') {
      return [
        {
          id: 'block',
          label: t('actions.blockIp'),
          icon: <BanGlassIcon size="1.4em" />,
          onSelect: () => blockIp(key),
        },
        {
          id: 'allow',
          label: t('actions.allowIp'),
          icon: (
            <CircleCheckGlassIcon className="text-tone-success" size="1.4em" />
          ),
          onSelect: () => allowIp(key),
        },
        {
          id: 'rule',
          label: t('actions.createRule'),
          icon: <PlusGlassIcon size="1.4em" />,
          onSelect: () => onCreateRule({ field: 'ip', value: key }),
        },
      ]
    }
    const field = TABLE_FIELD[table]
    if (!field) return []
    return [
      {
        id: 'rule',
        label: t('actions.createRule'),
        icon: <ExternalLinkGlassIcon size="1.4em" />,
        onSelect: () => onCreateRule({ field, value: key }),
      },
    ]
  }

  const dialog = (
    <ConfirmDialog
      open={lockout !== null}
      onOpenChange={(o) => {
        if (!o) setLockout(null)
      }}
      title={t('lockout.title')}
      description={t('lockout.description', { ip: lockout?.ip ?? '' })}
      confirmLabel={t('lockout.confirm')}
      tone="destructive"
      pending={block.isPending}
      onConfirm={() => lockout && blockIp(lockout.ip, lockout.opts, true)}
    />
  )

  return { menuFor, blockIp: (ip, opts) => blockIp(ip, opts), allowIp, dialog }
}
