import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Repo and provider access checks (internal/api/git_access_validate.go).

export type GitAccessCheckId =
  | 'repo_url'
  | 'credential'
  | 'access'
  | 'connection'
  | 'installation'
  | 'credentials'

export interface GitAccessCheck {
  id: GitAccessCheckId
  status: 'pass' | 'fail' | 'skipped'
  detail: string
}

export interface GitAccessFix {
  kind: 'auto' | 'guided' | 'manual'
  code: string
  hint: string
  link?: string
}

export interface GitAccessResult {
  ok: boolean
  credential?: string
  branch?: string
  head_sha?: string
  checks: GitAccessCheck[]
  fix?: GitAccessFix
}

export function useValidateGitAccess(path: string) {
  return useMutation({
    mutationFn: async (): Promise<GitAccessResult> => {
      const res = await fetch(path, { method: 'POST' })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `validate access failed: ${res.status}`),
        )
      }
      return (await res.json()) as GitAccessResult
    },
  })
}
