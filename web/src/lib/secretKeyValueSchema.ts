import { z } from 'zod'

export const secretKeyValueSchema = z.object({
  key: z.string().trim().min(1, 'Key is required'),
  value: z.string().min(1, 'Value is required'),
})

export type SecretKeyValueFormValues = z.infer<typeof secretKeyValueSchema>
