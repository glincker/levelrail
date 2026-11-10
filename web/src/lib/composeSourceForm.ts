import type {
  GitSourceResource,
  GitSourceTriggerMode,
} from '../types/gitSource'
import type { SetComposeSourceRequest } from '../queries/composeSource'
import { ApiError } from './apiError'

// Same default and byte cap as validateComposeRepoPath in the API.
export const DEFAULT_COMPOSE_PATH = 'compose.yaml'
const MAX_COMPOSE_PATH_BYTES = 512

export interface ComposeSourceForm {
  repoUrl: string
  branch: string
  composePath: string
  token: string
  triggerMode: GitSourceTriggerMode
}

export type ComposePathProblem =
  'tooLong' | 'control' | 'absolute' | 'parent' | 'notFile' | 'extension'

export type ComposeSourceErrorKey =
  | 'kindConflict'
  | 'protected'
  | 'frozen'
  | 'pollNotAllowed'
  | 'notDeployed'
  | 'noMasterKey'

export function emptyComposeSourceForm(): ComposeSourceForm {
  return {
    repoUrl: '',
    branch: 'main',
    composePath: DEFAULT_COMPOSE_PATH,
    token: '',
    triggerMode: 'push',
  }
}

export function composeFormFromResource(
  r: GitSourceResource,
): ComposeSourceForm {
  return {
    repoUrl: r.repo_url,
    branch: r.branch,
    composePath: r.compose_path ?? DEFAULT_COMPOSE_PATH,
    token: '',
    triggerMode: r.trigger_mode === 'release' ? 'release' : 'push',
  }
}

// Unicode Cc plus Cf, the two classes the server rejects.
const CONTROL_OR_FORMAT = /[\p{Cc}\p{Cf}]/u

// Mirrors the server rules so a bad path never costs a round trip.
export function composePathProblem(raw: string): ComposePathProblem | null {
  const p = raw.trim()
  if (p === '') return null
  if (new TextEncoder().encode(p).length > MAX_COMPOSE_PATH_BYTES) {
    return 'tooLong'
  }
  if (CONTROL_OR_FORMAT.test(p)) return 'control'
  if (p.includes('\\') || p.startsWith('/') || /^[A-Za-z]:/.test(p)) {
    return 'absolute'
  }
  if (p.split('/').includes('..')) return 'parent'
  const segments = p.split('/').filter((s) => s !== '' && s !== '.')
  if (segments.length === 0) return 'notFile'
  const last = segments[segments.length - 1] ?? ''
  if (!last.endsWith('.yml') && !last.endsWith('.yaml')) return 'extension'
  return null
}

export function composeSourcePayload(
  form: ComposeSourceForm,
): SetComposeSourceRequest {
  const req: SetComposeSourceRequest = {
    repo_url: form.repoUrl.trim(),
    branch: form.branch.trim() || undefined,
    compose_path: form.composePath.trim() || undefined,
    trigger_mode: form.triggerMode,
  }
  // An empty token keeps the stored one, so it is only sent when typed.
  const token = form.token.trim()
  if (token) req.token = token
  return req
}

// Maps the server refusals this card can explain to a translated message key.
export function composeSourceErrorKey(
  error: unknown,
): ComposeSourceErrorKey | null {
  if (!(error instanceof ApiError)) return null
  const msg = error.message.toLowerCase()
  if (error.status === 409) {
    return msg.includes('protected') ? 'protected' : 'kindConflict'
  }
  if (error.status === 423) return 'frozen'
  if (error.status === 501) return 'noMasterKey'
  if (error.status === 404 && msg.includes('deploy the stack once')) {
    return 'notDeployed'
  }
  if (error.status === 400 && msg.includes('trigger_mode')) {
    return 'pollNotAllowed'
  }
  return null
}
