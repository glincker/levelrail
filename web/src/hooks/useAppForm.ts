import { useEffect } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import {
  useForm,
  type FieldValues,
  type UseFormProps,
  type UseFormReturn,
} from 'react-hook-form'
import type { z } from 'zod'
import { focusFirstInvalid } from '../lib/focusInvalid'

type Options<I extends FieldValues, O extends FieldValues> = Omit<
  UseFormProps<I, unknown, O>,
  'resolver'
>

// useForm + zodResolver; errors show after blur (onTouched), then live. Any useForm option can override.
// After a failed submit, focus moves to the first aria-invalid control of the submitted form.
export function useAppForm<I extends FieldValues, O extends FieldValues = I>(
  schema: z.ZodType<O, I>,
  options: Options<I, O> = {},
): UseFormReturn<I, unknown, O> {
  const form = useForm<I, unknown, O>({
    mode: 'onTouched',
    reValidateMode: 'onChange',
    ...options,
    resolver: zodResolver(schema),
  })
  const { submitCount, errors } = form.formState
  const failed = submitCount > 0 && Object.keys(errors).length > 0
  useEffect(() => {
    if (!failed) return undefined
    // Wait a frame so aria-invalid is committed; the submit button or the
    // input Enter was pressed in still holds focus and names the form.
    const frame = requestAnimationFrame(() => {
      const origin = document.activeElement
      focusFirstInvalid(
        origin instanceof Element ? origin.closest('form') : null,
      )
    })
    return () => {
      cancelAnimationFrame(frame)
    }
  }, [failed, submitCount])
  return form
}
