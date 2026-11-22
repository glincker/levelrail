import type {
  AwsControlIdentity,
  CloudSpendReport,
  CloudSpendTotal,
} from '../types/cloudAccounts'

// Each currency keeps its own line; amounts are never added across currencies.
export function formatMoney(
  amount: number,
  currency: string,
  locale?: string,
): string {
  try {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency,
    }).format(amount)
  } catch {
    return `${amount.toFixed(2)} ${currency}`
  }
}

export function spendLines(
  totals: readonly CloudSpendTotal[],
  locale?: string,
): { currency: string; text: string }[] {
  return [...totals]
    .sort((a, b) => a.currency.localeCompare(b.currency))
    .map((t) => ({
      currency: t.currency,
      text: formatMoney(t.amount, t.currency, locale),
    }))
}

export type SpendBanner = 'disabled' | 'tag_not_active' | 'data_unavailable'

export function spendBanner(
  r: Pick<CloudSpendReport, 'state'>,
): SpendBanner | null {
  if (
    r.state === 'disabled' ||
    r.state === 'tag_not_active' ||
    r.state === 'data_unavailable'
  )
    return r.state
  return null
}

export function parseRetryAfter(value: string | null): number | null {
  if (value === null) return null
  const n = Number.parseInt(value, 10)
  return Number.isFinite(n) && n > 0 ? n : null
}

// STS reports a session ARN; the trust policy wants the IAM role it came from.
export function principalFromIdentity(
  id: AwsControlIdentity | undefined,
): string {
  if (!id?.configured || id.arn === '') return ''
  const m = /^arn:(aws[a-z-]*):sts::(\d{12}):assumed-role\/([^/]+)\/.+$/.exec(
    id.arn,
  )
  return m ? `arn:${m[1]}:iam::${m[2]}:role/${m[3]}` : id.arn
}
