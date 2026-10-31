import { useCallback, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  claimMailDomain,
  mailStepUpScope,
  useInvalidateMail,
  type MailDomain,
} from '../../queries/mailMailboxes'
import type { TeamResource } from '../../queries/teams'
import { useMailStatus } from '../../queries/mailStatus'
import { StepUpDialog } from '../env/StepUpDialog'
import { FIELD_CLASS, Reason } from './mailPanelUi'

interface Props {
  domains: MailDomain[]
  teams: TeamResource[]
  isRoot: boolean
}

/** Claim a domain the mail server already serves for one team. Root only. */
export function MailDomainClaim({ domains, teams, isRoot }: Props) {
  const { t } = useTranslation('mailServer')
  const invalidate = useInvalidateMail()
  const prefill = useMailStatus().data?.domain ?? ''
  const [domain, setDomain] = useState('')
  const [team, setTeam] = useState('')
  const [error, setError] = useState<string | null>(null)
  const onError = useCallback(
    (e: unknown) =>
      setError(e instanceof Error ? e.message : t('mailboxes.failed')),
    [t],
  )
  const gate = useStepUpGate(onError)
  const value = domain || prefill
  const teamId = team || teams.find((x) => x.default)?.id || teams[0]?.id || ''
  const claimed = domains.some((d) => d.domain === value)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    await gate.guard(mailStepUpScope.domain(value), async () => {
      await claimMailDomain(value, teamId)
      void invalidate()
    })
  }
  return (
    <div className="space-y-1">
      <form
        onSubmit={(e) => void submit(e)}
        className="flex flex-wrap items-end gap-2"
      >
        <div className="flex flex-col gap-1">
          <Label htmlFor="claim-domain" className="text-xs">
            {t('domains.domain')}
          </Label>
          <Input
            id="claim-domain"
            className="h-8 w-48"
            value={value}
            onChange={(e) => setDomain(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="claim-team" className="text-xs">
            {t('domains.team')}
          </Label>
          <select
            id="claim-team"
            className={FIELD_CLASS}
            value={teamId}
            onChange={(e) => setTeam(e.target.value)}
          >
            {teams.map((x) => (
              <option key={x.id} value={x.id}>
                {x.name}
              </option>
            ))}
          </select>
        </div>
        <Reason why={isRoot ? undefined : t('mailboxes.needsRoot')}>
          <Button
            type="submit"
            size="sm"
            variant="outline"
            disabled={!isRoot || !value || !teamId || claimed}
          >
            {claimed ? t('domains.claimed') : t('domains.claim')}
          </Button>
        </Reason>
      </form>
      {error && (
        <p role="alert" className="text-xs text-tone-danger">
          {error}
        </p>
      )}
      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={gate.cancel}
      />
    </div>
  )
}
