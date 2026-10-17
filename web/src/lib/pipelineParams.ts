import type { PipelineParameter } from '../types/pipelines'

// initialParamValues seeds the form with each parameter's default.
export function initialParamValues(
  params: PipelineParameter[],
): Record<string, string> {
  const out: Record<string, string> = {}
  for (const p of params) {
    out[p.name] = p.default ?? (p.type === 'boolean' ? 'false' : '')
  }
  return out
}
