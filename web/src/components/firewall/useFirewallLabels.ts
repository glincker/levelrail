import { useTranslation } from 'react-i18next'
import type {
  FwAction,
  FwField,
  FwMode,
  FwOp,
  FwOutcome,
} from '../../queries/firewallEdge'
import type { ActionDef, FieldDef, OperatorDef } from '../rules/ruleModel'
import { FW_ACTIONS, FW_FIELD_OPS, type FwMessages } from './firewallRuleModel'

/** useFirewallLabels returns translated names for every enum the firewall shows, with literal keys so tsc checks them. */
export function useFirewallLabels() {
  const { t } = useTranslation('firewall')

  const outcome: Record<FwOutcome, string> = {
    allowed: t('outcome.allowed'),
    denied: t('outcome.denied'),
    challenged: t('outcome.challenged'),
    logged: t('outcome.logged'),
    rate_limited: t('outcome.rate_limited'),
  }
  const action: Record<FwAction, string> = {
    allow: t('action.allow'),
    deny: t('action.deny'),
    challenge: t('action.challenge'),
    log: t('action.log'),
    rate_limit: t('action.rate_limit'),
  }
  const actionHint: Record<FwAction, string> = {
    allow: t('actionHint.allow'),
    deny: t('actionHint.deny'),
    challenge: t('actionHint.challenge'),
    log: t('actionHint.log'),
    rate_limit: t('actionHint.rate_limit'),
  }
  const mode: Record<FwMode, string> = {
    off: t('mode.off'),
    log: t('mode.log'),
    challenge: t('mode.challenge'),
    block: t('mode.block'),
  }
  const field: Record<FwField, string> = {
    path: t('field.path'),
    host: t('field.host'),
    method: t('field.method'),
    header: t('field.header'),
    query: t('field.query'),
    user_agent: t('field.user_agent'),
    ip: t('field.ip'),
    country: t('field.country'),
  }
  const op: Record<FwOp, string> = {
    equals: t('op.equals'),
    starts_with: t('op.starts_with'),
    contains: t('op.contains'),
    glob: t('op.glob'),
    in_cidr: t('op.in_cidr'),
    exists: t('op.exists'),
  }
  const reason: Record<string, string> = {
    rule: t('reason.rule'),
    rate_rule: t('reason.rate_rule'),
    ban: t('reason.ban'),
    bot: t('reason.bot'),
    ai_bot: t('reason.ai_bot'),
    attack_mode: t('reason.attack_mode'),
    shield_rate: t('reason.shield_rate'),
    shield_concurrency: t('reason.shield_concurrency'),
    shield_bot: t('reason.shield_bot'),
    shield_body: t('reason.shield_body'),
  }
  const auditAction: Record<string, string> = {
    'firewall.rule_created': t('audit.actions.rule_created'),
    'firewall.rule_updated': t('audit.actions.rule_updated'),
    'firewall.rule_deleted': t('audit.actions.rule_deleted'),
    'firewall.rules_reordered': t('audit.actions.rules_reordered'),
    'firewall.ip_blocked': t('audit.actions.ip_blocked'),
    'firewall.ip_allowed': t('audit.actions.ip_allowed'),
    'firewall.ban_lifted': t('audit.actions.ban_lifted'),
    'firewall.settings_changed': t('audit.actions.settings_changed'),
    'firewall.attack_mode_on': t('audit.actions.attack_mode_on'),
    'firewall.attack_mode_off': t('audit.actions.attack_mode_off'),
  }

  const messages: FwMessages = {
    nameRequired: t('validation.nameRequired'),
    noteTooLong: t('validation.noteTooLong'),
    actionRequired: t('validation.actionRequired'),
    rateCount: t('validation.rateCount'),
    rateWindow: t('validation.rateWindow'),
    paramName: t('validation.paramName'),
    valueRequired: t('validation.valueRequired'),
    ipInvalid: t('validation.ipInvalid'),
    methodInvalid: t('validation.methodInvalid'),
    countryInvalid: t('validation.countryInvalid'),
    pathSlash: t('validation.pathSlash'),
    wildcards: t('validation.wildcards'),
  }

  return {
    outcome,
    action,
    actionHint,
    mode,
    field,
    op,
    reason,
    auditAction,
    messages,
  }
}

const FIELD_ORDER: FwField[] = [
  'path',
  'host',
  'method',
  'header',
  'query',
  'user_agent',
  'ip',
  'country',
]
const NAMED: FwField[] = ['header', 'query']

/** The closed field and action catalogs the rule builder is given. Country only appears when a geo source exists. */
export function useFirewallCatalog(geoActive: boolean): {
  fields: FieldDef[]
  actions: ActionDef[]
} {
  const { t } = useTranslation('firewall')
  const labels = useFirewallLabels()
  const placeholders: Record<FwField, string> = {
    path: t('placeholder.path'),
    host: t('placeholder.host'),
    method: t('placeholder.method'),
    header: t('placeholder.header'),
    query: t('placeholder.query'),
    user_agent: t('placeholder.user_agent'),
    ip: t('placeholder.ip'),
    country: t('placeholder.country'),
  }
  const fields: FieldDef[] = FIELD_ORDER.filter(
    (f) => f !== 'country' || geoActive,
  ).map((f) => ({
    id: f,
    label: labels.field[f],
    named: NAMED.includes(f),
    namePlaceholder: NAMED.includes(f) ? t('placeholder.paramName') : undefined,
    valuePlaceholder: placeholders[f],
    operators: FW_FIELD_OPS[f].map((o): OperatorDef => ({
      id: o,
      label: labels.op[o],
      valueless: o === 'exists',
    })),
  }))
  const actions: ActionDef[] = FW_ACTIONS.map((a) => ({
    id: a,
    label: labels.action[a],
    hint: labels.actionHint[a],
    params:
      a === 'rate_limit'
        ? [
            {
              id: 'rate_count',
              label: t('rateLimit.count'),
              kind: 'number',
              min: 1,
              placeholder: '100',
              unit: t('rateLimit.countUnit'),
            },
            {
              id: 'rate_window_seconds',
              label: t('rateLimit.window'),
              kind: 'number',
              min: 1,
              placeholder: '60',
              unit: t('rateLimit.windowUnit'),
            },
          ]
        : undefined,
  }))
  return { fields, actions }
}
