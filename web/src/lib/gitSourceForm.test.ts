import { describe, expect, it } from 'vitest'
import {
  additionalServicesPayload,
  duplicateServiceNames,
  incompleteRowIndexes,
  incompleteRowNumbers,
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

  it('lists 1-based row numbers for the missing-name message', () => {
    expect(incompleteRowNumbers([0])).toBe('1')
    expect(incompleteRowNumbers([0, 2])).toBe('1, 3')
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
