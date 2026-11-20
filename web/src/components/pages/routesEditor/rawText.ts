import { newRedirect, type DraftRedirect } from '../rulesDraft'

/** One rule per line: "from to [status][!]". A trailing "!" on the status means force. */
export function redirectsToText(rows: readonly DraftRedirect[]): string {
  return rows
    .map((r) =>
      `${r.from} ${r.to} ${r.status}${r.force ? '!' : ''}`.replace(/\s+/g, ' '),
    )
    .join('\n')
}

/** Reuses ids by position so rows keep their React identity while typing. */
export function textToRedirects(
  text: string,
  prev: readonly DraftRedirect[],
): DraftRedirect[] {
  const out: DraftRedirect[] = []
  for (const line of text.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const [from = '', to = '', st = '301'] = trimmed.split(/\s+/)
    const force = st.endsWith('!')
    const status = /^\d{3}$/.test(st.replace('!', ''))
      ? st.replace('!', '')
      : '301'
    const base = prev[out.length] ?? newRedirect()
    out.push({ id: base.id, from, to, status, force })
  }
  return out
}

export type DiffOp = Readonly<{ kind: 'same' | 'add' | 'del'; line: string }>

/** Line diff via longest common subsequence; rule lists are short so O(n*m) is fine. */
export function diffLines(
  a: readonly string[],
  b: readonly string[],
): DiffOp[] {
  const dp: number[][] = Array.from({ length: a.length + 1 }, () =>
    new Array<number>(b.length + 1).fill(0),
  )
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      dp[i]![j] =
        a[i] === b[j]
          ? dp[i + 1]![j + 1]! + 1
          : Math.max(dp[i + 1]![j]!, dp[i]![j + 1]!)
    }
  }
  const ops: DiffOp[] = []
  let i = 0
  let j = 0
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      ops.push({ kind: 'same', line: a[i]! })
      i++
      j++
    } else if (dp[i + 1]![j]! >= dp[i]![j + 1]!) {
      ops.push({ kind: 'del', line: a[i++]! })
    } else {
      ops.push({ kind: 'add', line: b[j++]! })
    }
  }
  while (i < a.length) ops.push({ kind: 'del', line: a[i++]! })
  while (j < b.length) ops.push({ kind: 'add', line: b[j++]! })
  return ops
}
