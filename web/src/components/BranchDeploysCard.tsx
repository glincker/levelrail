import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  ExternalLinkGlassIcon,
  GitBranchGlassIcon,
  LoaderGlassIcon,
  TrashGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { EmptyState } from '@/components/ui/empty-state'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { StatusBadge } from '@/components/ui/status-badge'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { RelativeTime } from './kit'
import {
  useBranchDeploys,
  useDeleteBranchDeploy,
  useSetBranchDeploySettings,
} from '../queries/branchDeploys'
import type {
  BranchDeploy,
  BranchDeployMode,
  BranchDeployStatus,
} from '../types/branchDeploy'

const SHORT_SHA_LENGTH = 8

const STATUS_ICON: Record<BranchDeployStatus, UiIcon> = {
  deploying: LoaderGlassIcon,
  active: CircleCheckGlassIcon,
  failed: CircleAlertGlassIcon,
}

const STATUS_TONE: Record<
  BranchDeployStatus,
  'neutral' | 'success' | 'danger'
> = {
  deploying: 'neutral',
  active: 'success',
  failed: 'danger',
}

const MODES: BranchDeployMode[] = ['off', 'all', 'patterns']

function splitPatterns(text: string): string[] {
  return text
    .split(/[\n,]/)
    .map((p) => p.trim())
    .filter((p) => p !== '')
}

function BranchDeployRow({
  deploy,
  onDelete,
  deleting,
}: {
  deploy: BranchDeploy
  onDelete: (slug: string, close: () => void) => void
  deleting: boolean
}) {
  const { t } = useTranslation('branchDeploys')
  return (
    <li className="flex flex-wrap items-start justify-between gap-3 py-3">
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <GitBranchGlassIcon
            className="size-5 text-muted-foreground"
            aria-hidden="true"
          />
          <span className="truncate font-mono text-sm">{deploy.branch}</span>
          <StatusBadge
            tone={STATUS_TONE[deploy.status]}
            icon={STATUS_ICON[deploy.status]}
            label={t(`status.${deploy.status}`)}
          />
        </div>
        <p className="truncate text-xs text-muted-foreground">
          <span className="font-mono">
            {deploy.head_sha.slice(0, SHORT_SHA_LENGTH)}
          </span>
          {deploy.head_message ? ` ${deploy.head_message}` : ''}
        </p>
        {deploy.url ? (
          <p className="truncate font-mono text-xs">{deploy.url}</p>
        ) : (
          <p className="text-xs text-muted-foreground">{t('list.noUrl')}</p>
        )}
        {deploy.status === 'failed' && deploy.status_reason ? (
          <p className="text-xs text-destructive">{deploy.status_reason}</p>
        ) : null}
        {deploy.expires_at ? (
          <p className="text-xs text-muted-foreground">
            {t('list.expires')} <RelativeTime at={deploy.expires_at} live />
          </p>
        ) : null}
      </div>
      <div className="flex items-center gap-2">
        {deploy.url ? (
          <Button
            size="sm"
            variant="outline"
            nativeButton={false}
            aria-label={t('list.openLabel', { branch: deploy.branch })}
            render={<a href={deploy.url} target="_blank" rel="noreferrer" />}
          >
            <ExternalLinkGlassIcon size="1.4em" aria-hidden="true" />
            {t('list.open')}
          </Button>
        ) : null}
        <ConfirmAction
          title={t('deleteDialog.title', { branch: deploy.branch })}
          description={t('deleteDialog.description')}
          consequences={[t('deleteDialog.consequence')]}
          confirmLabel={t('deleteDialog.confirm')}
          pendingLabel={t('deleteDialog.pending')}
          pending={deleting}
          onConfirm={(close) => {
            onDelete(deploy.slug, close)
          }}
          trigger={(open) => (
            <Button
              size="sm"
              variant="outline"
              aria-label={t('list.deleteLabel', { branch: deploy.branch })}
              onClick={open}
            >
              <TrashGlassIcon size="1.4em" aria-hidden="true" />
              {t('list.delete')}
            </Button>
          )}
        />
      </div>
    </li>
  )
}

export function BranchDeploysCard({ appName }: { appName: string }) {
  const { t } = useTranslation('branchDeploys')
  const query = useBranchDeploys(appName)
  const save = useSetBranchDeploySettings(appName)
  const remove = useDeleteBranchDeploy(appName)

  const settings = query.data?.settings
  const [draft, setDraft] = useState<{
    mode: BranchDeployMode
    text: string
  } | null>(null)
  const mode = draft?.mode ?? settings?.mode ?? 'off'
  const patternText = draft?.text ?? settings?.patterns.join('\n') ?? ''

  function onSave() {
    save.mutate(
      { mode, patterns: splitPatterns(patternText) },
      {
        onSuccess: () => {
          setDraft(null)
          toast.add({ title: t('settings.saved'), type: 'success' })
        },
        onError: (err) => {
          toast.add({
            title: t('settings.saveError'),
            description: err.message,
            type: 'error',
          })
        },
      },
    )
  }

  function onDelete(slug: string, close: () => void) {
    remove.mutate(slug, {
      onSuccess: () => {
        toast.add({ title: t('deleteDialog.success'), type: 'success' })
        close()
      },
      onError: (err) => {
        toast.add({
          title: t('deleteDialog.error'),
          description: err.message,
          type: 'error',
        })
      },
    })
  }

  const modeLabel: Record<BranchDeployMode, string> = {
    off: t('settings.modeOff'),
    all: t('settings.modeAll'),
    patterns: t('settings.modePatterns'),
  }
  const deploys = query.data?.deploys ?? []

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="branch-deploy-mode">
              {t('settings.mode')}
            </FieldLabel>
            <Select
              value={mode}
              onValueChange={(value) => {
                if (value)
                  setDraft({
                    mode: value,
                    text: patternText,
                  })
              }}
            >
              <SelectTrigger
                id="branch-deploy-mode"
                className="w-full max-w-xs"
              >
                <SelectValue>{modeLabel[mode]}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                {MODES.map((m) => (
                  <SelectItem key={m} value={m}>
                    {modeLabel[m]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          {mode === 'patterns' ? (
            <Field>
              <FieldLabel htmlFor="branch-deploy-patterns">
                {t('settings.patterns')}
              </FieldLabel>
              <Textarea
                id="branch-deploy-patterns"
                className="min-h-24 font-mono"
                value={patternText}
                placeholder={t('settings.patternsPlaceholder')}
                onChange={(e) => {
                  setDraft({ mode, text: e.target.value })
                }}
              />
              <FieldHint>{t('settings.patternsHint')}</FieldHint>
            </Field>
          ) : null}
          {settings ? (
            <p className="text-xs text-muted-foreground">
              {settings.production_branch
                ? `${t('settings.productionNote', { branch: settings.production_branch })} `
                : ''}
              {t('settings.idleNote', { hours: settings.idle_ttl_hours })}
            </p>
          ) : null}
          <Button onClick={onSave} disabled={save.isPending || !settings}>
            {save.isPending ? t('settings.saving') : t('settings.save')}
          </Button>
        </div>

        <div>
          <h3 className="text-sm font-medium">{t('list.title')}</h3>
          {query.isError ? (
            <p role="alert" className="py-3 text-sm text-destructive">
              {t('list.loadError')}
            </p>
          ) : deploys.length === 0 && !query.isPending ? (
            <EmptyState
              compact
              icon={<GitBranchGlassIcon className="size-6" />}
              title={t('list.emptyTitle')}
              description={
                settings?.mode === 'off'
                  ? t('list.emptyOff')
                  : t('list.emptyOn')
              }
            />
          ) : (
            <ul className="divide-y">
              {deploys.map((d) => (
                <BranchDeployRow
                  key={d.slug}
                  deploy={d}
                  onDelete={onDelete}
                  deleting={remove.isPending}
                />
              ))}
            </ul>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
