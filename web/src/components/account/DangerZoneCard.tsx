import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { SettingsCardSkeleton } from '../settings/SettingsSkeletons'
import { StepUpDialog } from '../env/StepUpDialog'
import {
  AccountApiError,
  deleteCheckQueryOptions,
  useDeleteAccount,
  type DeleteBlocker,
} from '../../queries/accountSelf'

const STEP_UP_SCOPE = { action: 'manage', app: 'account-delete' } as const

function BlockerRow({ blocker }: { blocker: DeleteBlocker }) {
  const { t } = useTranslation('account')
  const teams = blocker.code === 'team_owner'
  return (
    <li className="flex flex-wrap items-center justify-between gap-3 text-sm">
      <span>
        {t(`danger.blockers.${blocker.code}`, {
          name: blocker.resource_name || t('danger.teamUnnamed'),
        })}
      </span>
      <Link
        to={teams ? '/settings/teams' : '/settings/users'}
        className={buttonVariants({ variant: 'outline', size: 'sm' })}
      >
        {teams ? t('danger.fixTeams') : t('danger.fixUsers')}
      </Link>
    </li>
  )
}

export function DangerZoneCard() {
  const { t } = useTranslation('account')
  const check = useQuery(deleteCheckQueryOptions())
  const del = useDeleteAccount()
  const [stepUp, setStepUp] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const email = check.data?.email ?? ''

  const remove = () => {
    setError(null)
    del.mutate(
      { confirm: email },
      {
        onSuccess: () => {
          toast.add({ title: t('danger.deleted'), type: 'success' })
          window.location.assign('/login')
        },
        onError: (e) => {
          if (e.code === 'step_up_required') {
            setConfirming(false)
            setStepUp(true)
            return
          }
          if (e.status === 409) {
            setConfirming(false)
            void check.refetch()
            return
          }
          setError(
            e.code === 'confirmation_mismatch'
              ? t('danger.mismatch')
              : e instanceof AccountApiError
                ? e.message
                : t('danger.deleteFailed'),
          )
        },
      },
    )
  }

  if (check.isPending) return <SettingsCardSkeleton rows={1} />

  return (
    <Card className="border-destructive/40">
      <CardHeader>
        <CardTitle className="text-destructive">{t('danger.title')}</CardTitle>
        <CardDescription>{t('danger.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {check.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
            />
            <AlertDescription className="flex items-center justify-between gap-3">
              <span>{t('danger.checkFailed')}</span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void check.refetch()}
              >
                {t('common.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : check.data.can_delete ? (
          <Button
            type="button"
            variant="destructive"
            size="sm"
            onClick={() => setStepUp(true)}
          >
            {t('danger.delete')}
          </Button>
        ) : (
          <Alert>
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
            />
            <AlertTitle>{t('danger.blockedTitle')}</AlertTitle>
            <AlertDescription className="space-y-3">
              <p>{t('danger.blockedIntro')}</p>
              <ul className="space-y-2">
                {check.data.blockers.map((b) => (
                  <BlockerRow
                    key={`${b.code}:${b.resource_id ?? ''}`}
                    blocker={b}
                  />
                ))}
              </ul>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={check.isFetching}
                onClick={() => void check.refetch()}
              >
                {t('danger.recheck')}
              </Button>
            </AlertDescription>
          </Alert>
        )}
      </CardContent>
      <StepUpDialog
        scope={stepUp ? STEP_UP_SCOPE : null}
        description={t('danger.dialogDescription')}
        onCancel={() => setStepUp(false)}
        onVerified={() => {
          setStepUp(false)
          setConfirming(true)
        }}
      />
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        tone="destructive"
        title={t('danger.dialogTitle')}
        description={t('danger.dialogDescription')}
        consequences={[
          t('danger.consequenceTokens'),
          t('danger.consequenceSessions'),
          t('danger.consequenceExport'),
        ]}
        requireTyped={email}
        confirmLabel={t('danger.confirm')}
        pendingLabel={t('danger.pending')}
        pending={del.isPending}
        error={error}
        onConfirm={remove}
      />
    </Card>
  )
}
