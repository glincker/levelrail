import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import {
  ActivityGlassIcon,
  CircleAlertGlassIcon,
  GitBranchGlassIcon,
  GlobeGlassIcon,
  MemoryStickGlassIcon,
  RotateCwGlassIcon,
  TerminalGlassIcon,
  WrenchGlassIcon,
} from '@thesvg/ui-react'
import { toast } from '@/components/ui/toast'
import { useAppPower } from '../apps/useAppPower'
import { useApplyPending } from '../../queries/appTimeline'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'
import type { SuggestionActionKind, SuggestionDescriptor } from './suggestions'

const ICONS: Record<SuggestionActionKind, React.ReactNode> = {
  open_health: <ActivityGlassIcon className="size-5.5" />,
  restart: <RotateCwGlassIcon className="size-5.5" />,
  apply_pending: <RotateCwGlassIcon className="size-5.5" />,
  open_logs: <TerminalGlassIcon className="size-5.5" />,
  show_fix: <WrenchGlassIcon className="size-5.5" />,
  open_deploy: <CircleAlertGlassIcon className="size-5.5 text-tone-danger" />,
  open_domains: <GlobeGlassIcon className="size-5.5" />,
  open_resources: <MemoryStickGlassIcon className="size-5.5" />,
  connect_git: <GitBranchGlassIcon className="size-5.5" />,
}

export function useSuggestionActions(app: AppDetail, latest?: DeployAttempt) {
  const { t } = useTranslation('common')
  const navigate = useNavigate()
  const power = useAppPower(app)
  const applyPending = useApplyPending(app.name)
  const [fixOpen, setFixOpen] = useState(false)
  const name = app.name
  const go = (to: string) => () => void navigate({ to, params: { name } })
  const onError = (e: Error) => toast.add({ title: e.message, type: 'error' })

  const run: Record<SuggestionActionKind, () => void> = {
    open_health: go('/apps/$name/health'),
    restart: power.restart,
    apply_pending: () =>
      applyPending.mutate(undefined, {
        onSuccess: () =>
          toast.add({ title: t('appPower.applying'), type: 'success' }),
        onError,
      }),
    open_logs: go('/apps/$name/logs'),
    show_fix: () => setFixOpen(true),
    open_deploy: () =>
      void navigate(
        latest
          ? {
              to: '/apps/$name/deploys/$deployId/logs',
              params: { name, deployId: latest.id },
            }
          : { to: '/apps/$name/deploys', params: { name } },
      ),
    open_domains: go('/apps/$name/domains'),
    open_resources: go('/apps/$name/resources'),
    connect_git: go('/apps/$name/source'),
  }
  const pending: Partial<Record<SuggestionActionKind, boolean>> = {
    restart: power.restarting,
    apply_pending: applyPending.isPending,
  }
  return {
    run,
    pending,
    fixOpen,
    setFixOpen,
    confirmDialog: power.confirmDialog,
  }
}

export function toSuggestionItems(
  descriptors: SuggestionDescriptor[],
  actions: Pick<ReturnType<typeof useSuggestionActions>, 'run' | 'pending'>,
  dismiss: (id: string) => void,
) {
  return descriptors.map((d) => ({
    id: d.id,
    tone: d.tone,
    icon: ICONS[d.action.kind],
    title: d.title,
    detail: d.detail,
    onDismiss: () => dismiss(d.id),
    actions: [
      {
        label: d.action.label,
        kind: 'primary' as const,
        pending: actions.pending[d.action.kind] ?? false,
        onClick: actions.run[d.action.kind],
      },
    ],
  }))
}
