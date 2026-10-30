import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { PencilGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useSetEnvironmentProtectedGlobal } from '../../queries/globalEnvironments'
import type { GlobalEnvironment } from '../../types/environment'
import { EnvironmentKindBadge } from './EnvironmentKindBadge'
import { EnvironmentFormDialog } from './EnvironmentFormDialog'
import { isBuiltIn } from '../../lib/environmentBuiltIn'
import { DeleteEnvironmentDialog } from './DeleteEnvironmentDialog'

export function EnvironmentsTable({
  environments,
}: {
  environments: GlobalEnvironment[]
}) {
  const { t } = useTranslation('environments')
  const setProtected = useSetEnvironmentProtectedGlobal()

  function toggle(env: GlobalEnvironment, next: boolean) {
    setProtected.mutate(
      { id: env.id, protectedFlag: next },
      {
        onSuccess: () => {
          toast.add({
            title: t(next ? 'settings.protectedOn' : 'settings.protectedOff', {
              name: env.name,
            }),
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: t('settings.updateFailed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  const columns = useMemo<ResponsiveColumn<GlobalEnvironment>[]>(
    () => [
      {
        id: 'name',
        header: t('settings.columns.name'),
        primary: true,
        cell: (env) => (
          <span className="flex flex-wrap items-center gap-2 font-medium">
            {env.name}
            {isBuiltIn(env) ? (
              <Badge variant="outline">{t('settings.builtIn')}</Badge>
            ) : null}
            {env.scope === 'project' ? (
              <Badge variant="muted">{t('settings.scopeProject')}</Badge>
            ) : null}
          </span>
        ),
      },
      {
        id: 'kind',
        header: t('settings.columns.kind'),
        cell: (env) => <EnvironmentKindBadge kind={env.kind} />,
      },
      {
        id: 'protected',
        header: t('settings.columns.protected'),
        cell: (env) => (
          <Switch
            checked={env.protected}
            disabled={setProtected.isPending}
            aria-label={t('settings.protectedToggle', { name: env.name })}
            onCheckedChange={(next) => {
              toggle(env, next)
            }}
          />
        ),
      },
      {
        id: 'apps',
        header: t('settings.columns.apps'),
        priority: 2,
        align: 'end',
        cell: (env) => <span className="tabular-nums">{env.app_count}</span>,
      },
      {
        id: 'databases',
        header: t('settings.columns.databases'),
        priority: 2,
        align: 'end',
        cell: (env) => (
          <span className="tabular-nums">{env.database_count}</span>
        ),
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [t, setProtected.isPending],
  )

  return (
    <ResponsiveTable
      rows={environments}
      columns={columns}
      getRowId={(env) => env.id}
      ariaLabel={t('settings.title')}
      renderActions={(env) => (
        <span className="inline-flex gap-1">
          <EnvironmentFormDialog
            environment={env}
            trigger={
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                aria-label={`${t('settings.edit')} ${env.name}`}
              >
                <PencilGlassIcon size="1.4em" />
              </Button>
            }
          />
          <DeleteEnvironmentDialog
            environment={env}
            others={environments}
            trigger={
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                disabled={isBuiltIn(env)}
                aria-label={`${t('settings.delete')} ${env.name}`}
              >
                <TrashGlassIcon size="1.4em" />
              </Button>
            }
          />
        </span>
      )}
    />
  )
}
