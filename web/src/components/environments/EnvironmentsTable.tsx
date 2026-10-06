import { useTranslation } from 'react-i18next'
import { PencilSimpleIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
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

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('settings.columns.name')}</TableHead>
          <TableHead>{t('settings.columns.kind')}</TableHead>
          <TableHead>{t('settings.columns.protected')}</TableHead>
          <TableHead className="text-right">
            {t('settings.columns.apps')}
          </TableHead>
          <TableHead className="text-right">
            {t('settings.columns.databases')}
          </TableHead>
          <TableHead className="text-right">
            {t('settings.columns.actions')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {environments.map((env) => {
          const reserved = isBuiltIn(env)
          return (
            <TableRow key={env.id}>
              <TableCell className="font-medium">
                <span className="flex flex-wrap items-center gap-2">
                  {env.name}
                  {reserved ? (
                    <Badge variant="outline">{t('settings.builtIn')}</Badge>
                  ) : null}
                  {env.scope === 'project' ? (
                    <Badge variant="muted">{t('settings.scopeProject')}</Badge>
                  ) : null}
                </span>
              </TableCell>
              <TableCell>
                <EnvironmentKindBadge kind={env.kind} />
              </TableCell>
              <TableCell>
                <Switch
                  checked={env.protected}
                  disabled={setProtected.isPending}
                  aria-label={t('settings.protectedToggle', { name: env.name })}
                  onCheckedChange={(next) => {
                    toggle(env, next)
                  }}
                />
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {env.app_count}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {env.database_count}
              </TableCell>
              <TableCell className="text-right">
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
                        <PencilSimpleIcon />
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
                        disabled={reserved}
                        aria-label={`${t('settings.delete')} ${env.name}`}
                      >
                        <TrashIcon />
                      </Button>
                    }
                  />
                </span>
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
