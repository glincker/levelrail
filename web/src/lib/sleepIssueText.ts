import type { TFunction } from 'i18next'
import type { SleepIssue } from '../queries/appSleep'

/** issueText returns an issue's title and detail, falling back to local copy for a legacy blocked_reason. */
export function issueText(
  t: TFunction<'deploys'>,
  issue: SleepIssue,
): { title: string; detail: string } {
  const legacy = (key: 'title' | 'detail') =>
    t(`appSleep.legacy.${issue.code}.${key}`, { defaultValue: '' })
  return {
    title: issue.title || legacy('title') || issue.code,
    detail: issue.detail || legacy('detail'),
  }
}
