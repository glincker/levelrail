import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import type { IacSchema } from './schemaToFields'

// The shipped schema, so the tests break when it drifts from what the form assumes.
export const realSchema = JSON.parse(
  readFileSync(
    resolve(
      import.meta.dirname,
      '../../../../docs/schemas/resources.schema.json',
    ),
    'utf8',
  ),
) as IacSchema
