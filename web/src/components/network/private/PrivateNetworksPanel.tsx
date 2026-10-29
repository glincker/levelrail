import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Share2GlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../../../hooks/useIsRoot'
import {
  useCreatePrivateNetwork,
  useDeletePrivateNetwork,
  useUpdatePrivateNetwork,
  type PrivateNetwork,
} from '../../../queries/privateNetworks'

function MemberPicker({
  apps,
  value,
  onChange,
}: {
  apps: string[]
  value: string[]
  onChange: (next: string[]) => void
}) {
  const { t } = useTranslation('privateNetworks')
  if (apps.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('networks.noApps')}</p>
    )
  }
  return (
    <div className="flex max-h-40 flex-wrap gap-x-4 gap-y-2 overflow-auto">
      {apps.map((name) => (
        <Label key={name} className="gap-2 font-normal">
          <Checkbox
            checked={value.includes(name)}
            onCheckedChange={(on) =>
              onChange(on ? [...value, name] : value.filter((v) => v !== name))
            }
          />
          {name}
        </Label>
      ))}
    </div>
  )
}

function CreateForm({ apps }: { apps: string[] }) {
  const { t } = useTranslation('privateNetworks')
  const create = useCreatePrivateNetwork()
  const [name, setName] = useState('')
  const [members, setMembers] = useState<string[]>([])
  const [deny, setDeny] = useState(false)

  function submit(e: React.FormEvent) {
    e.preventDefault()
    create.mutate(
      { name, default_deny: deny, members },
      {
        onSuccess: () => {
          toast.add({ title: t('networks.created', { name }), type: 'success' })
          setName('')
          setMembers([])
          setDeny(false)
        },
        onError: (err) => toast.add({ title: err.message, type: 'error' }),
      },
    )
  }

  return (
    <form
      onSubmit={submit}
      className="space-y-3 rounded-lg border border-border p-3"
    >
      <div className="space-y-1.5">
        <Label htmlFor="pn-name">{t('networks.name')}</Label>
        <Input
          id="pn-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t('networks.namePlaceholder')}
          pattern="[a-z0-9][a-z0-9\-]{0,39}"
          required
        />
        <p className="text-xs text-muted-foreground">
          {t('networks.nameHelp')}
        </p>
      </div>
      <div className="space-y-1.5">
        <Label>{t('networks.apps')}</Label>
        <MemberPicker apps={apps} value={members} onChange={setMembers} />
      </div>
      <div className="flex items-center gap-2">
        <Switch
          checked={deny}
          onCheckedChange={setDeny}
          aria-label={t('networks.defaultDeny')}
        />
        <div>
          <p className="text-sm font-medium">{t('networks.defaultDeny')}</p>
          <p className="text-xs text-muted-foreground">
            {t('networks.defaultDenyHelp')}
          </p>
        </div>
      </div>
      <Button type="submit" disabled={create.isPending || name === ''}>
        {t('networks.create')}
      </Button>
    </form>
  )
}

function NetworkRow({
  network,
  apps,
}: {
  network: PrivateNetwork
  apps: string[]
}) {
  const { t } = useTranslation('privateNetworks')
  const update = useUpdatePrivateNetwork()
  const remove = useDeletePrivateNetwork()
  const [members, setMembers] = useState(network.members)
  const [confirm, setConfirm] = useState(false)
  const dirty =
    members.slice().sort().join() !== network.members.slice().sort().join()

  const onError = (err: Error) =>
    toast.add({ title: err.message, type: 'error' })

  return (
    <li className="space-y-2 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <p className="text-sm font-medium">{network.name}</p>
          {network.default_deny ? (
            <Badge variant="warning">{t('networks.defaultDeny')}</Badge>
          ) : null}
        </div>
        <div className="flex items-center gap-3">
          <Switch
            checked={network.default_deny}
            aria-label={t('networks.defaultDeny')}
            onCheckedChange={(on) =>
              update.mutate(
                { id: network.id, default_deny: on, members: network.members },
                { onError },
              )
            }
          />
          <Button size="sm" variant="outline" onClick={() => setConfirm(true)}>
            {t('networks.delete')}
          </Button>
        </div>
      </div>
      <MemberPicker apps={apps} value={members} onChange={setMembers} />
      {dirty ? (
        <Button
          size="sm"
          disabled={update.isPending}
          onClick={() =>
            update.mutate(
              { id: network.id, default_deny: network.default_deny, members },
              { onError },
            )
          }
        >
          {t('networks.saveMembers')}
        </Button>
      ) : null}
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t('networks.deleteTitle', { name: network.name })}
        description={t('networks.deleteBody')}
        confirmLabel={t('networks.delete')}
        tone="destructive"
        pending={remove.isPending}
        onConfirm={() =>
          remove.mutate(network.id, {
            onSuccess: () => {
              setConfirm(false)
              toast.add({ title: t('networks.deleted'), type: 'success' })
            },
            onError,
          })
        }
      />
    </li>
  )
}

export function PrivateNetworksPanel({
  networks,
  apps,
}: {
  networks: PrivateNetwork[]
  apps: string[]
}) {
  const { t } = useTranslation('privateNetworks')
  const isRoot = useIsRoot()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('networks.title')}</CardTitle>
        <CardDescription>{t('page.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {isRoot ? <CreateForm apps={apps} /> : null}
        {networks.length === 0 ? (
          <EmptyState
            compact
            icon={<Share2GlassIcon className="size-5.5" />}
            title={t('networks.empty')}
            description={t('networks.emptyHint')}
          />
        ) : (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {networks.map((n) => (
              <NetworkRow
                key={`${n.id}:${n.members.join()}:${n.default_deny}`}
                network={n}
                apps={apps}
              />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
