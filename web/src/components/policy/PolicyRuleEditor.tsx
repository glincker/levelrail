import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  POLICY_RULE_IDS,
  type PolicyMode,
  type PolicyRuleId,
} from '../../queries/deployPolicies'
import { VALUED_RULES, isNumericRule, type RuleInputs } from './policyRules'

const MODES: PolicyMode[] = ['off', 'warn', 'enforce']

const PLACEHOLDER_KEYS = {
  allowed_registries: 'rules.allowed_registries.placeholder',
  forbid_host_mounts: 'rules.forbid_host_mounts.placeholder',
  required_labels: 'rules.required_labels.placeholder',
  max_memory_mb: 'rules.max_memory_mb.placeholder',
  max_cpu: 'rules.max_cpu.placeholder',
  max_replicas: 'rules.max_replicas.placeholder',
} as const

interface Props {
  modes: Partial<Record<PolicyRuleId, PolicyMode>>
  inputs: RuleInputs
  disabled: boolean
  onMode: (id: PolicyRuleId, mode: PolicyMode) => void
  onInput: (id: PolicyRuleId, value: string) => void
}

export function PolicyRuleEditor({
  modes,
  inputs,
  disabled,
  onMode,
  onInput,
}: Props) {
  const { t } = useTranslation('deployPolicies')
  return (
    <ul className="divide-y rounded-lg border">
      {POLICY_RULE_IDS.map((id) => {
        const title = t(`rules.${id}.title`)
        const hasValue = isNumericRule(id) || VALUED_RULES.includes(id)
        const mode = modes[id] ?? 'off'
        return (
          <li
            key={id}
            className="flex flex-wrap items-start justify-between gap-3 p-3"
          >
            <div className="min-w-0 flex-1 basis-56 space-y-1">
              <p className="text-sm font-medium">{title}</p>
              <p className="text-xs text-muted-foreground">
                {t(`rules.${id}.hint`)}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {hasValue && mode !== 'off' ? (
                <Input
                  aria-label={t('valueLabel', { rule: title })}
                  className="w-48"
                  disabled={disabled}
                  inputMode={isNumericRule(id) ? 'decimal' : 'text'}
                  placeholder={t(
                    PLACEHOLDER_KEYS[id as keyof typeof PLACEHOLDER_KEYS],
                  )}
                  value={inputs[id] ?? ''}
                  onChange={(e) => {
                    onInput(id, e.target.value)
                  }}
                />
              ) : null}
              <Select
                value={mode}
                disabled={disabled}
                onValueChange={(v) => {
                  onMode(id, v as PolicyMode)
                }}
              >
                <SelectTrigger
                  aria-label={t('modeLabel', { rule: title })}
                  className="w-32"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MODES.map((m) => (
                    <SelectItem key={m} value={m}>
                      {t(`mode.${m}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </li>
        )
      })}
    </ul>
  )
}
