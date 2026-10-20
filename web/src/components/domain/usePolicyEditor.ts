import { useMemo, useState } from 'react'
import {
  fieldErrorsFor,
  usePolicyPreview,
  useSavePolicy,
} from '../../queries/domainPolicies'
import type { PolicyKind, SpecByKind } from '../../queries/domainPolicyTypes'
import { useDebounced, usePolicyDraft } from './usePolicyDraft'

// usePolicyEditor wires one tab: a persisted draft, the save mutation, a
// debounced server preview of the draft, and inline field errors from both.
export function usePolicyEditor<K extends PolicyKind>(
  app: string,
  domain: string,
  kind: K,
  saved: NonNullable<SpecByKind[K]>,
) {
  const draftState = usePolicyDraft(app, domain, kind, saved)
  const save = useSavePolicy(app, domain, kind)
  const [method, setMethod] = useState('GET')
  const [path, setPath] = useState('/')
  const debounced = useDebounced(draftState.draft)
  const input = useMemo(
    () => ({ policy: { [kind]: debounced }, method, path }),
    [kind, debounced, method, path],
  )
  const preview = usePolicyPreview(app, domain, input)

  const errors = useMemo(() => {
    const out: Record<string, string> = {}
    for (const e of preview.data?.errors ?? []) {
      if (e.field.startsWith(kind + '.')) {
        out[e.field.slice(kind.length + 1)] = e.message
      }
    }
    return { ...out, ...fieldErrorsFor(save.error, kind) }
  }, [preview.data, save.error, kind])

  return {
    ...draftState,
    save,
    preview,
    errors,
    method,
    setMethod,
    path,
    setPath,
    onSave: () => save.mutate(draftState.draft),
  }
}
