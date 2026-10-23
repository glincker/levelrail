import { zodResolver } from '@hookform/resolvers/zod'
import {
  useForm,
  type FieldValues,
  type UseFormProps,
  type UseFormReturn,
} from 'react-hook-form'
import type { z } from 'zod'

type Options<I extends FieldValues, O extends FieldValues> = Omit<
  UseFormProps<I, unknown, O>,
  'resolver'
>

// useForm + zodResolver; errors show after blur (onTouched), then live. Any useForm option can override.
export function useAppForm<I extends FieldValues, O extends FieldValues = I>(
  schema: z.ZodType<O, I>,
  options: Options<I, O> = {},
): UseFormReturn<I, unknown, O> {
  return useForm<I, unknown, O>({
    mode: 'onTouched',
    reValidateMode: 'onChange',
    ...options,
    resolver: zodResolver(schema),
  })
}
