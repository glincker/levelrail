import { describe, expect, it } from 'vitest'
import {
  additionalServicesPayload,
  duplicateServiceNames,
  incompleteRowIndexes,
  incompleteRowsMessage,
  servicesPayload,
  servicesSpecYaml,
} from './gitSourceForm'

const row = (serviceName: string, buildPath = '', port = '') => ({
  serviceName,
  buildType: 'dockerfile' as const,
  buildPath,
  port,
})

describe('gitSourceForm helpers', () => {
  it('flags only touched rows that lack a name', () => {
    expect(incompleteRowIndexes([row(''), row('', './x'), row('web')])).toEqual(
      [1],
    )
  })

  it('formats the missing-name message for one or many rows', () => {
    expect(incompleteRowsMessage([0])).toBe('Row 1 is missing a service name.')
    expect(incompleteRowsMessage([0, 2])).toBe(
      'Rows 1, 3 are missing a service name.',
    )
  })

  it('omits empty payloads and trims names', () => {
    expect(additionalServicesPayload([row('')])).toBeUndefined()
    expect(servicesPayload([row(' web ', ' ./Dockerfile ', '80')])).toEqual({
      web: { build: { type: 'dockerfile', path: './Dockerfile' }, port: 80 },
    })
  })

  it('reports duplicates and skips yaml for them', () => {
    const rows = [row('a'), row('a')]
    expect([...duplicateServiceNames(rows)]).toEqual(['a'])
    expect(servicesSpecYaml(rows)).toBe('')
    expect(servicesSpecYaml([row('a', '', '3000')])).toContain('port: 3000')
  })
})
