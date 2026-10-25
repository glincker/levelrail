import { createContext, useContext, useId, useLayoutEffect } from 'react'

export interface FieldContextValue {
  invalid: boolean
  required: boolean
  // Id of the control that owns the label link, once one has claimed it.
  controlId: string | undefined
  describedBy: string | undefined
  claimControl: (uid: string, explicitId: string | undefined) => () => void
  setLabelFor: (id: string | undefined) => void
  registerDescription: (id: string, kind: 'hint' | 'error') => () => void
  ownerUid: string | null
}

export const FieldContext = createContext<FieldContextValue | null>(null)

export function useFieldContext(): FieldContextValue | null {
  return useContext(FieldContext)
}

interface ControlProps {
  id?: string
  required?: boolean
  'aria-invalid'?: boolean | 'true' | 'false' | 'grammar' | 'spelling'
  'aria-describedby'?: string
  'aria-required'?: boolean | 'true' | 'false'
}

interface ControlWiring {
  id?: string
  'aria-invalid'?: true
  'aria-describedby'?: string
  'aria-required'?: true
}

// Gives the first control inside a Field its label link, invalid state,
// described-by ids and required flag. Explicit props always win.
export function useFieldControl(props: ControlProps): ControlWiring {
  const ctx = useContext(FieldContext)
  const uid = useId()
  const explicitId = props.id
  const claim = ctx?.claimControl
  useLayoutEffect(() => claim?.(uid, explicitId), [claim, uid, explicitId])

  if (!ctx || ctx.ownerUid !== uid) return {}

  const wiring: ControlWiring = {}
  if (!explicitId) wiring.id = ctx.controlId
  if (props['aria-invalid'] === undefined && ctx.invalid) {
    wiring['aria-invalid'] = true
  }
  const ids = [props['aria-describedby'], ctx.describedBy]
    .join(' ')
    .split(/\s+/)
    .filter(Boolean)
  if (ids.length > 0) wiring['aria-describedby'] = [...new Set(ids)].join(' ')
  if (
    props['aria-required'] === undefined &&
    (props.required || ctx.required)
  ) {
    wiring['aria-required'] = true
  }
  return wiring
}
