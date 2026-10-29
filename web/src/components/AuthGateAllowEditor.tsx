import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  IdCardLanyardGlassIcon,
  PlusGlassIcon,
  UserGlassIcon,
  UsersGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import type { AuthGateRuleKind } from '../queries/domainAuthGate'

export interface AllowDraft {
  kind: AuthGateRuleKind
  subject: string
  label: string
}

const KINDS: AuthGateRuleKind[] = ['user', 'team', 'role']

const KIND_ICONS = {
  user: UserGlassIcon,
  team: UsersGlassIcon,
  role: IdCardLanyardGlassIcon,
} as const

// Allow list editor for the built-in gate provider. New entries carry what
// the operator typed; the server resolves emails and names to ids.
export function AuthGateAllowEditor({
  id,
  rules,
  onChange,
  disabled,
}: {
  id: string
  rules: AllowDraft[]
  onChange: (rules: AllowDraft[]) => void
  disabled: boolean
}) {
  const { t } = useTranslation('domains')
  const [kind, setKind] = useState<AuthGateRuleKind>('user')
  const [value, setValue] = useState('')

  function add() {
    const subject = value.trim()
    if (!subject) {
      return
    }
    const dup = rules.some(
      (r) =>
        r.kind === kind &&
        (r.subject === subject ||
          r.label.toLowerCase() === subject.toLowerCase()),
    )
    if (!dup) {
      onChange([...rules, { kind, subject, label: subject }])
    }
    setValue('')
  }

  return (
    <Field>
      <FieldLabel htmlFor={id}>{t('authGate.allowLabel')}</FieldLabel>
      {rules.length > 0 ? (
        <ul className="flex flex-wrap gap-1.5">
          {rules.map((r, i) => {
            const Icon = KIND_ICONS[r.kind]
            const label = r.label || r.subject
            return (
              <li key={`${r.kind}:${r.subject}`}>
                <Badge variant="muted" className="gap-1">
                  <Icon className="size-3" aria-hidden="true" />
                  <span className="sr-only">
                    {t(`authGate.allowKind.${r.kind}`)}
                  </span>
                  {label}
                  <button
                    type="button"
                    className="ml-0.5 rounded-sm hover:text-foreground"
                    onClick={() => onChange(rules.filter((_, j) => j !== i))}
                    disabled={disabled}
                    aria-label={t('authGate.allowRemove', { label })}
                  >
                    <XGlassIcon className="size-3.5" />
                  </button>
                </Badge>
              </li>
            )
          })}
        </ul>
      ) : (
        <Alert>
          <AlertDescription>{t('authGate.allowEmpty')}</AlertDescription>
        </Alert>
      )}
      <div className="flex flex-wrap items-center gap-2">
        <div
          role="radiogroup"
          aria-label={t('authGate.allowKindLabel')}
          className="flex gap-1"
        >
          {KINDS.map((k) => (
            <Button
              key={k}
              type="button"
              size="sm"
              role="radio"
              aria-checked={kind === k}
              variant={kind === k ? 'secondary' : 'ghost'}
              onClick={() => setKind(k)}
              disabled={disabled}
            >
              {t(`authGate.allowKind.${k}`)}
            </Button>
          ))}
        </div>
        <Input
          id={id}
          className="min-w-40 flex-1"
          autoComplete="off"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              add()
            }
          }}
          disabled={disabled}
          placeholder={t(`authGate.allowPlaceholder.${kind}`)}
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={add}
          disabled={disabled || value.trim() === ''}
        >
          <PlusGlassIcon className="size-4" />
          {t('authGate.allowAdd')}
        </Button>
      </div>
      <FieldDescription>{t('authGate.allowHelp')}</FieldDescription>
    </Field>
  )
}
