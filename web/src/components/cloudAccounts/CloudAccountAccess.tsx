import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { formatAbsoluteTime } from '../../lib/relativeTime'
import {
  useCloudGrants,
  useCreateCloudGrant,
  useRevokeCloudGrant,
} from '../../queries/cloudAccounts'

const ACTIONS = [
  'view',
  'operate',
  'view_spend',
  'manage_budget',
  'share',
] as const

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export function CloudAccountAccess({
  accountId,
  canShare,
}: Readonly<{ accountId: string; canShare: boolean }>) {
  const { t } = useTranslation('cloudAccounts')
  const uid = useId()
  const grants = useCloudGrants(accountId, canShare)
  const create = useCreateCloudGrant(accountId)
  const revoke = useRevokeCloudGrant(accountId)
  const [principalType, setPrincipalType] = useState('user')
  const [principalId, setPrincipalId] = useState('')
  const [actions, setActions] = useState<string[]>(['view'])
  const [expires, setExpires] = useState('')
  const [error, setError] = useState<string | null>(null)

  if (!canShare) {
    return (
      <p className="text-sm text-muted-foreground">{t('access.needsShare')}</p>
    )
  }

  const valid = principalId.trim() !== '' && actions.length > 0

  function add() {
    setError(null)
    create.mutate(
      {
        resource_kind: 'account',
        principal_type: principalType,
        principal_id: principalId.trim(),
        actions,
        expires_at: expires ? new Date(expires).toISOString() : undefined,
      },
      {
        onSuccess: () => {
          setPrincipalId('')
          setExpires('')
        },
        onError: (e) => {
          setError(messageOf(e))
        },
      },
    )
  }

  return (
    <div className="space-y-4">
      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          if (valid) add()
        }}
      >
        <div className="space-y-1">
          <Label htmlFor={`${uid}-type`}>{t('access.principalType')}</Label>
          <NativeSelect
            id={`${uid}-type`}
            value={principalType}
            onChange={(e) => {
              setPrincipalType(e.target.value)
            }}
          >
            <option value="user">{t('access.user')}</option>
            <option value="team">{t('access.team')}</option>
          </NativeSelect>
        </div>
        <div className="min-w-48 flex-1 space-y-1">
          <Label htmlFor={`${uid}-id`}>{t('access.principalId')}</Label>
          <Input
            id={`${uid}-id`}
            value={principalId}
            onChange={(e) => {
              setPrincipalId(e.target.value)
            }}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${uid}-exp`}>{t('access.expires')}</Label>
          <Input
            id={`${uid}-exp`}
            type="datetime-local"
            value={expires}
            onChange={(e) => {
              setExpires(e.target.value)
            }}
          />
        </div>
        <fieldset className="flex flex-wrap gap-x-3 gap-y-1">
          <legend className="sr-only">{t('access.actions')}</legend>
          {ACTIONS.map((a) => (
            <label key={a} className="flex items-center gap-1.5 text-sm">
              <Checkbox
                checked={actions.includes(a)}
                onCheckedChange={(on) => {
                  setActions((cur) =>
                    on ? [...cur, a] : cur.filter((x) => x !== a),
                  )
                }}
              />
              {t(`access.action.${a}`)}
            </label>
          ))}
        </fieldset>
        <Button type="submit" size="sm" disabled={!valid || create.isPending}>
          {t('access.add')}
        </Button>
      </form>
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {grants.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          {t('section.loading')}
        </p>
      ) : grants.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {t('access.loadFailed')}
        </p>
      ) : (grants.data ?? []).length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('access.empty')}</p>
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border">
          {(grants.data ?? []).map((g) => (
            <li
              key={g.id}
              className="flex flex-wrap items-center gap-x-4 gap-y-1 px-3 py-2 text-sm"
            >
              <span className="min-w-0 flex-1 truncate">
                <span className="text-xs text-muted-foreground">
                  {t(`access.${g.principal_type}`, {
                    defaultValue: g.principal_type,
                  })}
                </span>{' '}
                <span className="font-medium">{g.principal_id}</span>
              </span>
              <span className="text-xs text-muted-foreground">
                {g.actions
                  .map((a) => t(`access.action.${a}`, { defaultValue: a }))
                  .join(', ')}
              </span>
              <span className="text-xs text-muted-foreground">
                {g.expires_at
                  ? t('access.expiresAt', {
                      when: formatAbsoluteTime(g.expires_at),
                    })
                  : t('access.noExpiry')}
              </span>
              <Button
                variant="outline"
                size="xs"
                disabled={revoke.isPending}
                aria-label={t('access.revokeNamed', { name: g.principal_id })}
                onClick={() => {
                  revoke.mutate(g.id, {
                    onError: (e) => {
                      setError(messageOf(e))
                    },
                  })
                }}
              >
                {t('access.revoke')}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
