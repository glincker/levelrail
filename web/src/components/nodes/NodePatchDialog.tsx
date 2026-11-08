import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
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
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  isTerminalRun,
  useNodePatchApply,
  useNodePatchPreview,
  useNodePatchRuns,
  type PatchPackage,
  type PatchRun,
} from '../../queries/nodePatch'
import { PatchRunDetails, PatchRunStatusBadge } from './NodePatchRunDetails'
import {
  needsNewPreview,
  patchErrorCode,
  patchErrorMessage,
} from './patchErrors'

const CONFIRM_WORD = 'APPLY'

function formatRemaining(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000))
  const m = Math.floor(total / 60)
  const s = String(total % 60).padStart(2, '0')
  return `${m}:${s}`
}

function PackageFlags({ pkg }: { pkg: PatchPackage }) {
  const { t } = useTranslation('nodes')
  return (
    <span className="flex flex-wrap gap-1">
      {pkg.security ? (
        <Badge variant="destructive">{t('patch.dialog.badgeSecurity')}</Badge>
      ) : null}
      {pkg.kernel ? (
        <Badge variant="warning">{t('patch.dialog.badgeKernel')}</Badge>
      ) : null}
      {pkg.container_runtime ? (
        <Badge variant="warning">{t('patch.dialog.badgeRuntime')}</Badge>
      ) : null}
    </span>
  )
}

function PackageTable({
  packages,
  muted,
}: {
  packages: PatchPackage[]
  muted?: boolean
}) {
  const { t } = useTranslation('nodes')
  return (
    <div className="max-h-56 overflow-auto rounded-md border border-border">
      <table
        className={`w-full text-xs ${muted ? 'text-muted-foreground' : ''}`}
      >
        <thead className="sticky top-0 bg-muted text-left text-muted-foreground">
          <tr>
            <th className="px-2 py-1 font-medium">
              {t('patch.dialog.colName')}
            </th>
            <th className="px-2 py-1 font-medium">
              {t('patch.dialog.colVersion')}
            </th>
            <th className="px-2 py-1 font-medium">
              {t('patch.dialog.colFlags')}
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {packages.map((p) => (
            <tr key={p.name}>
              <td className="px-2 py-1 font-mono">{p.name}</td>
              <td className="px-2 py-1 font-mono">
                {p.current_version ? `${p.current_version} → ` : ''}
                {p.new_version}
              </td>
              <td className="px-2 py-1">
                <PackageFlags pkg={p} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function ToggleRow({
  id,
  label,
  warning,
  checked,
  onChange,
}: {
  id: string
  label: string
  warning: string
  checked: boolean
  onChange: (value: boolean) => void
}) {
  return (
    <div className="flex items-start gap-3">
      <Switch
        id={id}
        size="sm"
        checked={checked}
        onCheckedChange={onChange}
        aria-describedby={`${id}-hint`}
        className="mt-0.5"
      />
      <div className="space-y-0.5">
        <label htmlFor={id} className="text-sm font-medium">
          {label}
        </label>
        <p id={`${id}-hint`} className="text-xs text-muted-foreground">
          {warning}
        </p>
      </div>
    </div>
  )
}

interface Props {
  nodeId: string
  nodeName: string
  onClose: () => void
}

// Review, confirm and watch one patch run; mounted only while open so it stays a lazy chunk.
export default function NodePatchDialog({ nodeId, nodeName, onClose }: Props) {
  const { t } = useTranslation('nodes')
  const preview = useNodePatchPreview(nodeId)
  const apply = useNodePatchApply(nodeId)
  const [includeAll, setIncludeAll] = useState(false)
  const [reboot, setReboot] = useState(false)
  const [typed, setTyped] = useState('')
  const [now, setNow] = useState(() => Date.now())
  const [started, setStarted] = useState<PatchRun | null>(null)

  const { mutate: runPreview } = preview
  useEffect(() => {
    runPreview({ include_all: false, refresh: true })
  }, [runPreview])

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [])

  const runs = useNodePatchRuns(nodeId, started !== null)
  const live = started
    ? (runs.data?.find((r) => r.id === started.id) ?? started)
    : null

  const data = preview.data
  const remaining = data ? new Date(data.expires_at).getTime() - now : 0
  const expired = data !== undefined && remaining <= 0
  const typedOk =
    typed.trim() === CONFIRM_WORD || typed.trim() === nodeName.trim()
  const canApply =
    data !== undefined &&
    !expired &&
    data.packages.length > 0 &&
    typedOk &&
    !apply.isPending

  function refresh(nextIncludeAll: boolean) {
    apply.reset()
    setTyped('')
    runPreview({ include_all: nextIncludeAll, refresh: true })
  }

  function toggleHeld(value: boolean) {
    setIncludeAll(value)
    refresh(value)
  }

  function submit() {
    if (!data) return
    apply.mutate(
      {
        preview_id: data.preview_id,
        confirm: true,
        include_all: includeAll,
        reboot_if_required: reboot,
      },
      { onSuccess: (run) => setStarted(run) },
    )
  }

  function goToDrains() {
    onClose()
    document.getElementById('node-drains')?.scrollIntoView()
  }

  const errorSource = apply.error ?? preview.error

  return (
    <Dialog open onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {live
              ? t('patch.dialog.progress')
              : t('patch.dialog.title', { name: nodeName })}
          </DialogTitle>
          <DialogDescription>{t('patch.dialog.description')}</DialogDescription>
        </DialogHeader>

        {live ? (
          <div className="space-y-3" aria-live="polite">
            <div className="flex items-center gap-2">
              <PatchRunStatusBadge status={live.status} />
              {!isTerminalRun(live.status) ? (
                <span className="text-xs text-muted-foreground">
                  {t('patch.dialog.reloadingNote')}
                </span>
              ) : null}
            </div>
            <PatchRunDetails run={live} />
          </div>
        ) : (
          <div className="space-y-3">
            {errorSource ? (
              <Alert variant="destructive" role="alert">
                <TriangleAlertGlassIcon
                  className="text-tone-warning"
                  size="1.7em"
                />
                <AlertDescription className="space-y-2">
                  <p>{patchErrorMessage(t, errorSource)}</p>
                  {patchErrorCode(errorSource) === 'drain_required' ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={goToDrains}
                    >
                      {t('patch.dialog.openDrains')}
                    </Button>
                  ) : null}
                  {needsNewPreview(errorSource) ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => refresh(includeAll)}
                    >
                      {t('patch.dialog.refresh')}
                    </Button>
                  ) : null}
                </AlertDescription>
              </Alert>
            ) : null}

            {preview.isPending && !data ? (
              <Skeleton className="h-32 w-full" />
            ) : null}

            {data ? (
              <>
                <p className="text-xs text-muted-foreground">
                  {t('patch.dialog.manager', { manager: data.manager })}
                </p>
                <section className="space-y-1">
                  <h3 className="text-xs font-medium">
                    {t('patch.dialog.packages')}
                  </h3>
                  {data.packages.length === 0 ? (
                    <p className="text-sm text-muted-foreground">
                      {t('patch.dialog.noPackages')}
                    </p>
                  ) : (
                    <PackageTable packages={data.packages} />
                  )}
                </section>

                {data.held.length > 0 || data.other_available > 0 ? (
                  <section className="space-y-1 rounded-md bg-muted/40 p-2">
                    <h3 className="text-xs font-medium text-muted-foreground">
                      {t('patch.dialog.held')}
                    </h3>
                    <p className="text-xs text-muted-foreground">
                      {t('patch.dialog.heldHint', {
                        count: data.other_available,
                      })}
                    </p>
                    {data.held.length > 0 ? (
                      <PackageTable packages={data.held} muted />
                    ) : null}
                  </section>
                ) : null}

                <ToggleRow
                  id="patch-include-held"
                  label={t('patch.dialog.includeHeld')}
                  warning={t('patch.dialog.includeHeldWarn')}
                  checked={includeAll}
                  onChange={toggleHeld}
                />
                <ToggleRow
                  id="patch-reboot"
                  label={t('patch.dialog.reboot')}
                  warning={t('patch.dialog.rebootWarn')}
                  checked={reboot}
                  onChange={setReboot}
                />
                {data.reboot_required_now ? (
                  <p className="text-xs text-tone-warning">
                    {t('patch.dialog.rebootNow')}
                  </p>
                ) : data.reboot_likely ? (
                  <p className="text-xs text-tone-warning">
                    {t('patch.dialog.rebootLikely')}
                  </p>
                ) : null}

                <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                  <div className="flex min-w-48 flex-1 items-center gap-2">
                    <label htmlFor="patch-confirm" className="text-xs">
                      {t('patch.dialog.typeToConfirm', { name: nodeName })}
                    </label>
                    <Input
                      id="patch-confirm"
                      value={typed}
                      autoComplete="off"
                      placeholder={t('patch.dialog.confirmPlaceholder')}
                      onChange={(e) => setTyped(e.target.value)}
                      className="h-8 max-w-40"
                    />
                  </div>
                  <span
                    className={
                      expired
                        ? 'text-xs text-tone-danger'
                        : 'text-xs text-muted-foreground'
                    }
                  >
                    {expired
                      ? t('patch.dialog.expired')
                      : t('patch.dialog.expiresIn', {
                          time: formatRemaining(remaining),
                        })}
                  </span>
                </div>
              </>
            ) : null}
          </div>
        )}

        <DialogFooter>
          {live ? (
            <Button type="button" variant="outline" onClick={onClose}>
              {t('patch.dialog.close')}
            </Button>
          ) : (
            <>
              {expired ? (
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => refresh(includeAll)}
                >
                  {t('patch.dialog.refresh')}
                </Button>
              ) : null}
              <Button type="button" variant="outline" onClick={onClose}>
                {t('patch.dialog.cancel')}
              </Button>
              <Button type="button" disabled={!canApply} onClick={submit}>
                {apply.isPending
                  ? t('patch.dialog.applying')
                  : t('patch.dialog.apply')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
