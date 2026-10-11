import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useFinishSignIn } from '../queries/auth'
import { pollLoginApproval } from '../queries/signIn'
import { NewDeviceApprovalWait } from './NewDeviceApprovalWait'
import { TwoFactorVerifyForm } from './TwoFactorVerifyForm'
import { Alert, AlertDescription } from './ui/alert'
import { Button } from './ui/button'

// An OAuth sign-in held for approval lands on /login?approval=<id>; the
// first poll, bound to this browser's cookie, supplies the number to show.
export function ResumedApprovalWait({
  approvalId,
  onBack,
  onUseCode,
}: {
  approvalId: string
  onBack: () => void
  onUseCode: () => void
}) {
  const { t } = useTranslation('signIn')
  const finish = useFinishSignIn()
  const first = useQuery({
    queryKey: ['auth', 'approval-resume', approvalId],
    queryFn: pollLoginApproval,
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
  })
  const data = first.data
  useEffect(() => {
    if (data?.status === 'approved' && data.username) {
      finish(data.username)
    }
  }, [data, finish])
  if (first.isPending) {
    return null
  }
  if (data?.status === 'approved' && data.mfa_required && data.mfa_token) {
    return <TwoFactorVerifyForm mfaToken={data.mfa_token} onBack={onBack} />
  }
  if (data?.status !== 'pending' || !data.expires_at) {
    return (
      <div className="space-y-3">
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>
            {data?.status === 'denied'
              ? t('approval.denied')
              : t('approval.expired')}
          </AlertDescription>
        </Alert>
        <Button
          type="button"
          variant="link"
          className="w-full"
          onClick={onBack}
        >
          {t('approval.back')}
        </Button>
      </div>
    )
  }
  return (
    <NewDeviceApprovalWait
      approvalId={approvalId}
      expiresAt={data.expires_at}
      matchNumber={data.match_number ?? 0}
      onBack={onBack}
      onUseCode={onUseCode}
    />
  )
}
