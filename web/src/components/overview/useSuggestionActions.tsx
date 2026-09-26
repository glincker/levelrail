import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import {
  ArrowClockwiseIcon,
  GitBranchIcon,
  GlobeIcon,
  HeartbeatIcon,
  MemoryIcon,
  TerminalWindowIcon,
  WarningCircleIcon,
  WrenchIcon,
} from '@phosphor-icons/react/dist/ssr'
import { toast } from '@/components/ui/toast'
import {
  HEALTH_CHECK_DEFAULT_PATH,
  healthCheckFrom,
} from '../../lib/healthCheckDefaults'
import { useRestartApp, useUpdateApp } from '../../queries/apps'
import { useApplyPending } from '../../queries/appTimeline'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'
import type { SuggestionActionKind, SuggestionDescriptor } from './suggestions'

const ICONS: Record<SuggestionActionKind, React.ReactNode> = {
  add_health: <HeartbeatIcon className="size-5" />,
  restart: <ArrowClockwiseIcon className="size-5" />,
  apply_pending: <ArrowClockwiseIcon className="size-5" />,
  open_logs: <TerminalWindowIcon className="size-5" />,
  show_fix: <WrenchIcon className="size-5" />,
  open_deploy: <WarningCircleIcon className="size-5" />,
  open_domains: <GlobeIcon className="size-5" />,
  open_resources: <MemoryIcon className="size-5" />,
  connect_git: <GitBranchIcon className="size-5" />,
}

export function useSuggestionActions(app: AppDetail, latest?: DeployAttempt) {
  const navigate = useNavigate()
  const restart = useRestartApp()
  const update = useUpdateApp(app.name)
  const applyPending = useApplyPending(app.name)
  const [fixOpen, setFixOpen] = useState(false)
  const name = app.name
  const go = (to: string) => () => void navigate({ to, params: { name } })
  const onError = (e: Error) => toast.add({ title: e.message, type: 'error' })

  const run: Record<SuggestionActionKind, () => void> = {
    add_health: () =>
      update.mutate(
        { ...app, health: healthCheckFrom(true, HEALTH_CHECK_DEFAULT_PATH) },
        {
          onSuccess: () =>
            toast.add({
              title: `Health check added on ${HEALTH_CHECK_DEFAULT_PATH}.`,
              type: 'success',
            }),
          onError,
        },
      ),
    restart: () =>
      restart.mutate(name, {
        onSuccess: () =>
          toast.add({ title: `Restarting "${name}".`, type: 'success' }),
        onError,
      }),
    apply_pending: () =>
      applyPending.mutate(undefined, {
        onSuccess: () =>
          toast.add({ title: 'Applying changes.', type: 'success' }),
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
    add_health: update.isPending,
    restart: restart.isPending,
    apply_pending: applyPending.isPending,
  }
  return { run, pending, fixOpen, setFixOpen }
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
