import { normalizeRepoUrl, type NormalizedRepoOption } from './gitRepoOptions'

/** Repositories matching the typed text, newest push first; a pasted URL matches only its own repository. */
export function filterRepos(
  repos: NormalizedRepoOption[],
  query: string,
  urlCandidate: string | null,
): NormalizedRepoOption[] {
  const sorted = [...repos].sort((a, b) =>
    (b.pushedAt ?? '').localeCompare(a.pushedAt ?? ''),
  )
  if (urlCandidate) {
    const want = normalizeRepoUrl(urlCandidate)
    return sorted.filter((r) => normalizeRepoUrl(r.cloneUrl) === want)
  }
  const q = query.trim().toLowerCase()
  if (q === '') return sorted
  return sorted.filter(
    (r) =>
      r.displayPath.toLowerCase().includes(q) ||
      (r.language ?? '').toLowerCase().includes(q),
  )
}
