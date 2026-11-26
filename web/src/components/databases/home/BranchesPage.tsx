import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { ApiError } from '@/lib/apiError'
import { SchemaDiffDrawer } from './SchemaDiffDrawer'
import { orderTree } from './branchTree'
import { ACTION_STATES, expiryAllowed, parseGoDuration } from './branchTtl'
import { useRuntimeSettings } from '../../../queries/runtimeSettings'
import { formatBytes } from '../../../lib/format'
import {
  useBranchAction,
  useBranchCapabilities,
  useCreateBranch,
  useDatabaseBranches,
  useProtectBranch,
  type BranchAction,
  type BranchResource,
} from '../../../queries/databaseBranches'

const MODES = ['full', 'schema', 'anonymized'] as const
const EXPIRIES = ['1d', '7d', '30d', 'never'] as const
const READY_STATES = new Set(['ready', 'promoted'])

export function CreateBranchDialog({
  database,
  branches,
  open,
  onOpenChange,
}: Readonly<{
  database: string
  branches: BranchResource[]
  open: boolean
  onOpenChange: (open: boolean) => void
}>) {
  const { t } = useTranslation('dbBranches')
  const caps = useBranchCapabilities(database).data
  const create = useCreateBranch(database)
  const minTtlMs = parseGoDuration(
    useRuntimeSettings().data?.settings.find(
      (x) => x.key === 'branches.min_ttl',
    )?.value,
  )
  const [name, setName] = useState('')
  const [mode, setMode] = useState<string>('full')
  const [ttl, setTtl] = useState<string>('7d')
  const [parent, setParent] = useState('')
  const [protect, setProtect] = useState(false)

  function submit(e: React.FormEvent) {
    e.preventDefault()
    create.mutate(
      {
        name: name.trim(),
        mode,
        ttl,
        parent: parent || undefined,
        protected: protect,
      },
      {
        onSuccess: () => {
          toast.add({ title: t('created'), type: 'success' })
          setName('')
          onOpenChange(false)
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} className="space-y-3">
          <DialogHeader>
            <DialogTitle>{t('create.title')}</DialogTitle>
            <DialogDescription>{t('create.description')}</DialogDescription>
          </DialogHeader>
          <div className="grid items-center gap-2 sm:grid-cols-[7rem_1fr]">
            <label htmlFor="branch-name" className="text-sm">
              {t('create.name')}
            </label>
            <Input
              id="branch-name"
              value={name}
              required
              autoComplete="off"
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
            <label htmlFor="branch-mode" className="text-sm">
              {t('create.mode')}
            </label>
            <NativeSelect
              id="branch-mode"
              value={mode}
              onChange={(e) => {
                setMode(e.target.value)
              }}
            >
              {MODES.map((m) => {
                const cap = caps?.modes[m]
                return (
                  <option key={m} value={m} disabled={cap?.available === false}>
                    {t(`mode.${m}`)}
                    {cap?.available === false && cap.reason
                      ? ` (${cap.reason})`
                      : ''}
                  </option>
                )
              })}
            </NativeSelect>
            <label htmlFor="branch-parent" className="text-sm">
              {t('create.parent')}
            </label>
            <NativeSelect
              id="branch-parent"
              value={parent}
              onChange={(e) => {
                setParent(e.target.value)
              }}
            >
              <option value="">{t('create.parentRoot')}</option>
              {branches
                .filter((b) => READY_STATES.has(b.state))
                .map((b) => (
                  <option key={b.name} value={b.name}>
                    {b.name}
                  </option>
                ))}
            </NativeSelect>
            <label htmlFor="branch-ttl" className="text-sm">
              {t('create.expiry')}
            </label>
            <NativeSelect
              id="branch-ttl"
              value={ttl}
              onChange={(e) => {
                setTtl(e.target.value)
              }}
            >
              {EXPIRIES.map((x) => (
                <option
                  key={x}
                  value={x}
                  disabled={!expiryAllowed(x, minTtlMs)}
                >
                  {t(`expiry.${x}`)}
                </option>
              ))}
            </NativeSelect>
            <span className="text-sm">{t('create.protect')}</span>
            <Switch
              checked={protect}
              aria-label={t('create.protect')}
              onCheckedChange={setProtect}
            />
          </div>
          {create.error ? (
            <Alert variant="destructive">
              <AlertDescription>{create.error.message}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                onOpenChange(false)
              }}
            >
              {t('cancel')}
            </Button>
            <Button
              type="submit"
              disabled={name.trim() === '' || create.isPending}
            >
              {t('create.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ConfirmDialog({
  database,
  target,
  onClose,
}: Readonly<{
  database: string
  target: { branch: string; action: BranchAction } | null
  onClose: () => void
}>) {
  const { t } = useTranslation('dbBranches')
  const run = useBranchAction(database)
  const [typed, setTyped] = useState('')
  if (!target) return null
  return (
    <Dialog
      open
      onOpenChange={(o) => {
        if (!o) {
          setTyped('')
          onClose()
        }
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t(`confirm.${target.action}.title`)}</DialogTitle>
          <DialogDescription>
            {t(`confirm.${target.action}.body`, { name: target.branch })}
          </DialogDescription>
        </DialogHeader>
        <label className="space-y-1 text-sm">
          <span>{t('confirm.typeName', { name: target.branch })}</span>
          <Input
            value={typed}
            autoComplete="off"
            onChange={(e) => {
              setTyped(e.target.value)
            }}
          />
        </label>
        {run.error ? (
          <Alert variant="destructive">
            <AlertDescription>
              {run.error instanceof ApiError && run.error.status === 409
                ? t('notReady')
                : run.error.message}
            </AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('cancel')}
          </Button>
          <Button
            variant={target.action === 'promote' ? 'default' : 'destructive'}
            disabled={typed !== target.branch || run.isPending}
            onClick={() => {
              run.mutate(target, {
                onSuccess: () => {
                  setTyped('')
                  onClose()
                },
              })
            }}
          >
            {t(`actions.${target.action}`)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function stateChip(state: string): string {
  return ['cloning', 'ready', 'failed', 'expiring'].includes(state)
    ? state
    : 'working'
}

/** BranchesPage lists the branch tree of one database and creates, resets, promotes and deletes branches. */
export function BranchesPage({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('dbBranches')
  const { data, isLoading, error } = useDatabaseBranches(name)
  const protect = useProtectBranch(name)
  const [createOpen, setCreateOpen] = useState(false)
  const [diffBranch, setDiffBranch] = useState<string | null>(null)
  const [target, setTarget] = useState<{
    branch: string
    action: BranchAction
  } | null>(null)
  const rows = useMemo(() => orderTree(data?.branches ?? []), [data])
  const unavailable = error instanceof ApiError && error.status === 404

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
            {t('title')}
          </h2>
          <p className="text-sm text-muted-foreground">{t('description')}</p>
        </div>
        <Button
          onClick={() => {
            setCreateOpen(true)
          }}
          disabled={unavailable}
        >
          {t('create.open')}
        </Button>
      </div>
      {isLoading ? (
        <Skeleton className="h-32 w-full" />
      ) : unavailable ? (
        <Alert>
          <AlertDescription>{t('unavailable')}</AlertDescription>
        </Alert>
      ) : error ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('loadFailed', { error: error.message })}
          </AlertDescription>
        </Alert>
      ) : rows.length === 0 ? (
        <p className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
          {t('empty')}
        </p>
      ) : (
        <ul className="divide-y rounded-lg border">
          {rows.map(({ branch: b, depth }) => (
            <li
              key={b.name}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-sm"
            >
              <span
                className="flex min-w-0 items-center gap-2"
                data-depth={depth}
              >
                {depth > 0 ? (
                  <span aria-hidden="true" className="text-muted-foreground">
                    {'└'.padStart(depth + 1, ' ')}
                  </span>
                ) : null}
                <span className="truncate font-medium">{b.name}</span>
              </span>
              <Badge variant="muted">
                {t(`mode.${b.mode}` as 'mode.full', { defaultValue: b.mode })}
              </Badge>
              <Badge
                variant={b.state === 'failed' ? 'destructive' : 'muted'}
                title={b.state_reason}
              >
                {t(`state.${stateChip(b.state)}` as 'state.ready')}
              </Badge>
              {b.protected ? (
                <Badge variant="outline">{t('protected')}</Badge>
              ) : null}
              <span className="text-xs text-muted-foreground">
                {b.expires_at
                  ? t('expires', {
                      date: new Date(b.expires_at).toLocaleString(),
                    })
                  : t('expiry.never')}
                {b.size_bytes > 0 ? ` · ${formatBytes(b.size_bytes)}` : ''}
              </span>
              <span className="ml-auto flex flex-wrap items-center gap-1">
                <label className="flex items-center gap-1 text-xs">
                  <Switch
                    checked={b.protected}
                    aria-label={t('protectAria', { name: b.name })}
                    onCheckedChange={(v) => {
                      protect.mutate({ branch: b.name, protect: v })
                    }}
                  />
                  {t('protectLabel')}
                </label>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={!READY_STATES.has(b.state)}
                  onClick={() => {
                    setDiffBranch(b.name)
                  }}
                >
                  {t('schemaDiff.open')}
                </Button>
                {(['reset', 'promote', 'delete'] as const).map((a) => (
                  <Button
                    key={a}
                    size="sm"
                    variant="outline"
                    disabled={!ACTION_STATES[a].includes(b.state)}
                    onClick={() => {
                      setTarget({ branch: b.name, action: a })
                    }}
                  >
                    {t(`actions.${a}`)}
                  </Button>
                ))}
              </span>
            </li>
          ))}
        </ul>
      )}
      <CreateBranchDialog
        database={name}
        branches={data?.branches ?? []}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />
      <SchemaDiffDrawer
        database={name}
        branch={diffBranch}
        branches={data?.branches ?? []}
        onClose={() => {
          setDiffBranch(null)
        }}
      />
      <ConfirmDialog
        database={name}
        target={target}
        onClose={() => {
          setTarget(null)
        }}
      />
    </div>
  )
}
