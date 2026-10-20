import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { UseFormReset, UseFormWatch } from 'react-hook-form'
import { useDebouncedValue } from './useDebouncedValue'

const DRAFT_SAVE_DELAY_MS = 600

function readDraft<T>(storageKey: string): Partial<T> | null {
  try {
    const raw = window.localStorage.getItem(storageKey)
    return raw ? (JSON.parse(raw) as Partial<T>) : null
  } catch {
    // Storage can throw or hold corrupt JSON in private-browsing/storage-
    // restricted contexts; a blank form is always a safe fallback.
    return null
  }
}

function writeDraft<T>(storageKey: string, value: T): void {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(value))
  } catch {
    // A blocked or full store just means this draft never persists, never
    // a reason to break the form itself.
  }
}

function removeDraft(storageKey: string): void {
  try {
    window.localStorage.removeItem(storageKey)
  } catch {
    // Nothing to clean up if storage already isn't working.
  }
}

function withoutKeys<T extends Record<string, unknown>>(
  value: T,
  keys: readonly (keyof T)[],
): Partial<T> {
  if (keys.length === 0) return value
  const copy: Partial<T> = { ...value }
  for (const key of keys) delete copy[key]
  return copy
}

// Debounced localStorage autosave/restore for a create-resource step-2
// form: as the operator types, values are written to `storageKey` once
// they settle, so a refresh or crash mid-fill doesn't lose the form (the
// "draft mode" ask alongside the CreateResourceWizard overflow fix).
// Restore is keyed off `open` transitioning to true rather than mount
// alone, since CreateAppDialog/CreateDatabaseDialog keep their field
// components mounted across repeated open/close cycles, unlike
// CreateResourceWizard's step 2, which remounts fresh via `key={selected}`
// every time.
export function useFormDraft<T extends Record<string, unknown>>({
  storageKey,
  open,
  watch,
  reset,
  defaultValues,
  excludeKeys = [],
}: {
  storageKey: string
  open: boolean
  watch: UseFormWatch<T>
  reset: UseFormReset<T>
  defaultValues: T
  /** Fields never written to storage, e.g. a token or password field the
   *  rest of the form is otherwise safe to persist. */
  excludeKeys?: readonly (keyof T)[]
}) {
  const [restoredFromDraft, setRestoredFromDraft] = useState(false)
  // Whether the restore effect below has run at least once for the
  // current `open` cycle, regardless of whether it actually found a
  // draft. Gates the write effect further down: without it, the write
  // effect's very first pass (queued from the pre-restore render, before
  // the restore effect's reset() call takes effect) would see the
  // form's untouched blank defaults, decide there's nothing to save, and
  // delete a draft that's about to be restored into the form a moment
  // later. Plain state, not a ref: each render's own effect closure needs
  // to see the value as of THAT render, which only state (not a ref,
  // whose mutations are visible to every closure immediately) provides.
  const [hasCheckedStorage, setHasCheckedStorage] = useState(false)
  const defaultsRef = useRef(defaultValues)
  // Refs can't be mutated during render (react-hooks/refs), so the
  // "latest defaultValues" ref updates here instead, in an effect with no
  // dependency array so it runs after every render, ahead of the restore
  // effect below (layout effects declared earlier commit first).
  useLayoutEffect(() => {
    defaultsRef.current = defaultValues
  })

  useLayoutEffect(() => {
    if (!open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setHasCheckedStorage(false)
      return
    }
    const saved = readDraft<T>(storageKey)
    if (saved && Object.keys(saved).length > 0) {
      reset({ ...defaultsRef.current, ...saved })
      // Synchronizing with an external system (localStorage) on the open
      // transition, then reflecting what was found: the sanctioned effect
      // shape, not the "derive state from props" case exhaustive-deps/
      // set-state-in-effect otherwise steers away from.
      setRestoredFromDraft(true)
    }
    setHasCheckedStorage(true)
    // Only reacting to open/storageKey transitions, not to reset/
    // defaultValues identity churn on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, storageKey])

  const values = watch()
  const debouncedValues = useDebouncedValue(values, DRAFT_SAVE_DELAY_MS)

  useEffect(() => {
    if (!open || !hasCheckedStorage) return
    // useDebouncedValue's own initial state is whatever `values` was on
    // its first render, so right after a restore's reset() call (a value
    // change with no keystroke to debounce), `debouncedValues` briefly
    // lags the live, already-restored `values` by up to one debounce
    // interval. Waiting for the two to agree avoids writing or, worse,
    // clearing the draft with that stale, not-yet-caught-up value.
    if (JSON.stringify(values) !== JSON.stringify(debouncedValues)) return
    const toSave = withoutKeys(debouncedValues, excludeKeys)
    const baseline = withoutKeys(defaultsRef.current, excludeKeys)
    if (JSON.stringify(toSave) === JSON.stringify(baseline)) {
      removeDraft(storageKey)
      return
    }
    writeDraft(storageKey, toSave)
    // excludeKeys is a literal array at each call site, not worth adding
    // as a dependency.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [values, debouncedValues, open, hasCheckedStorage, storageKey])

  function discardDraft() {
    removeDraft(storageKey)
    reset(defaultsRef.current)
    setRestoredFromDraft(false)
  }

  function dismissDraftNotice() {
    setRestoredFromDraft(false)
  }

  function clearDraft() {
    removeDraft(storageKey)
    setRestoredFromDraft(false)
  }

  return { restoredFromDraft, discardDraft, dismissDraftNotice, clearDraft }
}
