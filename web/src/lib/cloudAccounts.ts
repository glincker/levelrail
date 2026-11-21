// Pure helpers for the cloud accounts UI: wizard step rules and status mapping.
import type { StatusState } from '@/components/kit/tone'

export const AWS_CAPABILITIES = [
  'inventory',
  'deploy',
  'cost',
  's3',
  'dns',
  'destructive',
] as const

export type AwsCapability = (typeof AWS_CAPABILITIES)[number]

// Destructive stays off until someone asks for it; the rest match the common path.
export const DEFAULT_CAPABILITIES: readonly AwsCapability[] =
  AWS_CAPABILITIES.filter((c) => c !== 'destructive')

export const AWS_STEPS = ['scope', 'kit', 'connect'] as const
export type AwsStep = (typeof AWS_STEPS)[number]

export interface AwsWizardState {
  capabilities: readonly AwsCapability[]
  regionsText: string
  trustedPrincipalArn: string
  guardPolicyArn: string
  roleName: string
  region: string
  roleArn: string
  label: string
}

export const INITIAL_WIZARD: AwsWizardState = {
  capabilities: DEFAULT_CAPABILITIES,
  regionsText: '',
  trustedPrincipalArn: '',
  guardPolicyArn: '',
  roleName: '',
  region: 'us-east-1',
  roleArn: '',
  label: 'AWS',
}

const REGION_RE = /^[a-z]{2}(-[a-z]+)+-\d$/
const PRINCIPAL_RE = /^arn:aws[a-z-]*:iam::\d{12}:(root|role\/.+|user\/.+)$/
const ROLE_RE = /^arn:aws[a-z-]*:iam::\d{12}:role\/.+$/
const POLICY_RE = /^arn:aws[a-z-]*:iam::(\d{12}|aws):policy\/.+$/

export function toggleCapability(
  current: readonly AwsCapability[],
  cap: AwsCapability,
): AwsCapability[] {
  return current.includes(cap)
    ? current.filter((c) => c !== cap)
    : AWS_CAPABILITIES.filter((c) => c === cap || current.includes(c))
}

export function parseRegions(text: string): string[] {
  const seen = new Set<string>()
  for (const part of text.split(/[\s,]+/)) {
    if (part !== '') seen.add(part.toLowerCase())
  }
  return [...seen]
}

export function invalidRegions(regions: readonly string[]): string[] {
  return regions.filter((r) => !REGION_RE.test(r))
}

export function isRegion(value: string): boolean {
  return REGION_RE.test(value)
}

export function isRoleArn(value: string): boolean {
  return ROLE_RE.test(value.trim())
}

export function isPrincipalArn(value: string): boolean {
  return PRINCIPAL_RE.test(value.trim())
}

export function isPolicyArn(value: string): boolean {
  return POLICY_RE.test(value.trim())
}

export type ScopeIssue =
  'noCapabilities' | 'badRegions' | 'badPrincipal' | 'badGuard' | 'badRegion'

// First step is valid when the kit can be generated; the server re-checks everything.
export function scopeIssues(s: AwsWizardState): ScopeIssue[] {
  const out: ScopeIssue[] = []
  if (s.capabilities.length === 0) out.push('noCapabilities')
  if (invalidRegions(parseRegions(s.regionsText)).length > 0) {
    out.push('badRegions')
  }
  if (!isPrincipalArn(s.trustedPrincipalArn)) out.push('badPrincipal')
  if (s.guardPolicyArn.trim() !== '' && !isPolicyArn(s.guardPolicyArn)) {
    out.push('badGuard')
  }
  if (!isRegion(s.region)) out.push('badRegion')
  return out
}

export function canLeaveStep(
  step: AwsStep,
  s: AwsWizardState,
  hasKit: boolean,
): boolean {
  if (step === 'scope') return scopeIssues(s).length === 0
  if (step === 'kit') return hasKit
  return isRoleArn(s.roleArn) && s.label.trim() !== ''
}

export function nextStep(step: AwsStep): AwsStep {
  const i = AWS_STEPS.indexOf(step)
  return AWS_STEPS[Math.min(i + 1, AWS_STEPS.length - 1)] ?? step
}

export function prevStep(step: AwsStep): AwsStep {
  const i = AWS_STEPS.indexOf(step)
  return AWS_STEPS[Math.max(i - 1, 0)] ?? step
}

export function accountStatusState(status: string): StatusState {
  if (status === 'active') return 'healthy'
  if (status === 'error') return 'failed'
  return 'paused'
}
