import {
  POLICY_RULE_IDS,
  type PolicyMode,
  type PolicyRuleId,
  type PolicyRules,
} from '../../queries/deployPolicies'

const NUMERIC: PolicyRuleId[] = ['max_memory_mb', 'max_cpu', 'max_replicas']
export const VALUED_RULES: PolicyRuleId[] = [
  'allowed_registries',
  'forbid_host_mounts',
  'required_labels',
]

export function isNumericRule(id: PolicyRuleId): boolean {
  return NUMERIC.includes(id)
}

/** Editable text for a rule's value or limit input. */
export type RuleInputs = Partial<Record<PolicyRuleId, string>>

export function inputsFromRules(rules: PolicyRules): RuleInputs {
  const out: RuleInputs = {}
  for (const id of POLICY_RULE_IDS) {
    const r = rules[id]
    if (!r) continue
    out[id] = isNumericRule(id)
      ? String(r.limit ?? '')
      : (r.values ?? []).join(', ')
  }
  return out
}

/** Builds the rule set to send, or the id of the first rule with a bad value. */
export function rulesFromInputs(
  modes: Partial<Record<PolicyRuleId, PolicyMode>>,
  inputs: RuleInputs,
): { rules: PolicyRules } | { badRule: PolicyRuleId } {
  const rules: PolicyRules = {}
  for (const id of POLICY_RULE_IDS) {
    const mode = modes[id]
    if (!mode || mode === 'off') continue
    const raw = (inputs[id] ?? '').trim()
    if (isNumericRule(id)) {
      const n = Number(raw)
      if (!Number.isFinite(n) || n <= 0) return { badRule: id }
      rules[id] = { mode, limit: n }
    } else if (VALUED_RULES.includes(id)) {
      const values = raw
        .split(',')
        .map((v) => v.trim())
        .filter((v) => v !== '')
      if (values.length === 0 && id !== 'forbid_host_mounts') {
        return { badRule: id }
      }
      rules[id] = { mode, values }
    } else {
      rules[id] = { mode }
    }
  }
  return { rules }
}
