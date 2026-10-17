import type { GitRepoSourceValue } from '../components/GitRepoSourcePicker'

const KEY = 'import-repo-draft'

export interface ImportDraft {
  step: 'source' | 'findings' | 'review'
  source: Omit<GitRepoSourceValue, 'token'>
  baseDirectory: string
}

/** Saves the resumable part of the import flow. Tokens and env values are never stored. */
export function saveImportDraft(draft: ImportDraft): void {
  try {
    sessionStorage.setItem(KEY, JSON.stringify(draft))
  } catch {
    // storage can be blocked; the flow still works without resume
  }
}

export function loadImportDraft(): ImportDraft | null {
  try {
    const raw = sessionStorage.getItem(KEY)
    if (!raw) return null
    const d = JSON.parse(raw) as Partial<ImportDraft>
    if (
      (d.step === 'source' || d.step === 'findings' || d.step === 'review') &&
      d.source?.repoUrl
    ) {
      return {
        step: d.step,
        source: d.source,
        baseDirectory: d.baseDirectory ?? '',
      }
    }
  } catch {
    return null
  }
  return null
}

export function clearImportDraft(): void {
  try {
    sessionStorage.removeItem(KEY)
  } catch {
    // nothing to clear when storage is unavailable
  }
}
