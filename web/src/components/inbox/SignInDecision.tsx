import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  signInKeys,
  useDecideApproval,
  type SignInRequests,
} from '../../queries/signIn'

export function SignInDecision({
  id,
  onDone,
}: {
  id: string
  onDone?: () => void
}) {
  const { t } = useTranslation('inbox')
  const qc = useQueryClient()
  const decide = useDecideApproval()
  const [match, setMatch] = useState('')
  const num = Number.parseInt(match, 10)
  const valid = num >= 10 && num <= 99

  const submit = (approve: boolean) => {
    const prev = qc.getQueryData<SignInRequests>(signInKeys.requests)
    if (prev) {
      qc.setQueryData<SignInRequests>(signInKeys.requests, {
        ...prev,
        approvals: prev.approvals.filter((a) => a.id !== id),
      })
    }
    decide.mutate(
      { id, approve, match: approve ? num : undefined },
      {
        onSuccess: onDone,
        onError: () => {
          if (prev) qc.setQueryData(signInKeys.requests, prev)
          toast.add({ title: t('page.actionError'), type: 'error' })
        },
      },
    )
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Input
        inputMode="numeric"
        maxLength={2}
        className="w-20"
        aria-label={t('page.matchLabel')}
        placeholder={t('page.matchPlaceholder')}
        value={match}
        onChange={(e) => {
          setMatch(e.target.value.replace(/\D/g, ''))
        }}
      />
      <Button
        size="sm"
        disabled={!valid || decide.isPending}
        onClick={() => {
          submit(true)
        }}
      >
        {t('page.approve')}
      </Button>
      <Button
        size="sm"
        variant="outline"
        disabled={decide.isPending}
        onClick={() => {
          submit(false)
        }}
      >
        {t('page.deny')}
      </Button>
    </div>
  )
}
