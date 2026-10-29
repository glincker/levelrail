import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import type { SessionOnLimit, SessionPolicy } from '@/queries/sessionPolicy'
import {
  useClearSessionPolicy,
  useSaveSessionPolicy,
} from '@/queries/sessionPolicy'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'

/** SessionPolicyForm edits one team's session policy inside its sheet. */
export function SessionPolicyForm({
  teamId,
  policy,
  onDone,
}: {
  teamId: string
  policy?: SessionPolicy
  onDone: () => void
}) {
  const { t } = useTranslation('sso')
  const save = useSaveSessionPolicy()
  const clear = useClearSessionPolicy()
  const [idle, setIdle] = useState(policy?.idle_timeout ?? '')
  const [maxAge, setMaxAge] = useState(policy?.max_lifetime ?? '')
  const [allow, setAllow] = useState((policy?.ip_allowlist ?? []).join('\n'))
  const [mfa, setMfa] = useState(policy?.require_mfa ?? false)
  const [maxSessions, setMaxSessions] = useState(
    String(policy?.max_concurrent_sessions ?? 0),
  )
  const [onLimit, setOnLimit] = useState<SessionOnLimit>(
    policy?.on_limit ?? 'evict_oldest',
  )
  const capped = Number(maxSessions) > 0
  const id = (s: string) => `policy-${teamId}-${s}`
  const error = save.error ?? clear.error

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(
          {
            teamId,
            input: {
              idle_timeout: idle.trim(),
              max_lifetime: maxAge.trim(),
              ip_allowlist: allow
                .split(/[\n,]/)
                .map((s) => s.trim())
                .filter(Boolean),
              require_mfa: mfa,
              max_concurrent_sessions: Math.max(0, Number(maxSessions) || 0),
              on_limit: onLimit,
            },
          },
          {
            onSuccess: () => {
              toast.add({ title: t('policy.saved'), type: 'success' })
              onDone()
            },
          },
        )
      }}
    >
      <FieldGroup className="gap-4">
        <Field>
          <FieldLabel htmlFor={id('idle')}>{t('policy.idle')}</FieldLabel>
          <Input
            id={id('idle')}
            className="font-mono"
            value={idle}
            onChange={(e) => setIdle(e.target.value)}
          />
          <FieldDescription>{t('policy.idleHelp')}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor={id('max')}>{t('policy.maxAge')}</FieldLabel>
          <Input
            id={id('max')}
            className="font-mono"
            value={maxAge}
            onChange={(e) => setMaxAge(e.target.value)}
          />
          <FieldDescription>{t('policy.maxAgeHelp')}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor={id('cap')}>{t('policy.maxSessions')}</FieldLabel>
          <Input
            id={id('cap')}
            type="number"
            min={0}
            max={1000}
            inputMode="numeric"
            className="font-mono"
            value={maxSessions}
            onChange={(e) => setMaxSessions(e.target.value)}
          />
          <FieldDescription>{t('policy.maxSessionsHelp')}</FieldDescription>
        </Field>
        {capped ? (
          <Field>
            <FieldLabel htmlFor={id('on-limit')}>
              {t('policy.onLimit')}
            </FieldLabel>
            <Select<string>
              value={onLimit}
              onValueChange={(value: string | null) => {
                setOnLimit(value === 'reject' ? 'reject' : 'evict_oldest')
              }}
            >
              <SelectTrigger id={id('on-limit')} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="evict_oldest">
                  {t('policy.onLimitEvict')}
                </SelectItem>
                <SelectItem value="reject">
                  {t('policy.onLimitReject')}
                </SelectItem>
              </SelectContent>
            </Select>
            <FieldDescription>{t('policy.onLimitHelp')}</FieldDescription>
          </Field>
        ) : null}
        <Field>
          <FieldLabel htmlFor={id('allow')}>{t('policy.allowlist')}</FieldLabel>
          <Textarea
            id={id('allow')}
            rows={5}
            className="font-mono text-xs"
            value={allow}
            onChange={(e) => setAllow(e.target.value)}
          />
          <FieldDescription>{t('policy.allowlistHelp')}</FieldDescription>
        </Field>
        <Field>
          <div className="flex items-center justify-between gap-3">
            <FieldLabel htmlFor={id('mfa')} className="font-normal">
              {t('policy.requireMfa')}
            </FieldLabel>
            <Switch id={id('mfa')} checked={mfa} onCheckedChange={setMfa} />
          </div>
          <FieldDescription>{t('policy.requireMfaHelp')}</FieldDescription>
        </Field>
      </FieldGroup>
      {error ? (
        <p role="alert" className="text-sm text-tone-danger">
          {error.message}
        </p>
      ) : null}
      <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-4">
        {policy ? (
          <Button
            type="button"
            variant="destructive"
            disabled={clear.isPending}
            onClick={() =>
              clear.mutate(teamId, {
                onSuccess: () => {
                  toast.add({ title: t('policy.cleared'), type: 'success' })
                  onDone()
                },
              })
            }
          >
            <TrashGlassIcon size="1.4em" aria-hidden="true" />
            {t('policy.clear')}
          </Button>
        ) : (
          <span />
        )}
        <Button type="submit" disabled={save.isPending}>
          {t('policy.save')}
        </Button>
      </div>
    </form>
  )
}
