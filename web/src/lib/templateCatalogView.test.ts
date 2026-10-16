import { describe, expect, it } from 'vitest'
import type { ServiceTemplateListItem } from '../queries/serviceTemplates'
import {
  ALL_CATEGORIES,
  countCategories,
  filterTemplates,
  memoryTier,
  sortTemplates,
} from './templateCatalogView'

function tpl(
  id: string,
  name: string,
  category: string,
  slogan = '',
  mem?: number,
): ServiceTemplateListItem {
  return {
    id,
    name,
    category,
    slogan,
    documentation_url: '',
    requires_configuration: false,
    recommended_memory_bytes: mem,
  }
}

const MIB = 1024 * 1024
const CATALOG = [
  tpl('grafana', 'Grafana', 'Monitoring', 'Dashboards for metrics', 512 * MIB),
  tpl('cafe', 'Café Bar', 'Productivity', 'Order coffee', 128 * MIB),
  tpl('redis', 'Redis', 'Database Tools', 'Fast cache for grafana data'),
  tpl('ghost', 'Ghost', 'Productivity', 'Publishing', 256 * MIB),
]

describe('filterTemplates', () => {
  const cases: [string, string, string, string[]][] = [
    [
      'empty input keeps all',
      '',
      ALL_CATEGORIES,
      ['grafana', 'cafe', 'redis', 'ghost'],
    ],
    [
      'whitespace only keeps all',
      '   ',
      ALL_CATEGORIES,
      ['grafana', 'cafe', 'redis', 'ghost'],
    ],
    ['by name', 'ghost', ALL_CATEGORIES, ['ghost']],
    ['by description', 'metrics', ALL_CATEGORIES, ['grafana']],
    [
      'name and description both hit',
      'grafana',
      ALL_CATEGORIES,
      ['grafana', 'redis'],
    ],
    ['diacritics folded from query', 'cafe', ALL_CATEGORIES, ['cafe']],
    ['diacritics folded from data', 'CAFÉ', ALL_CATEGORIES, ['cafe']],
    ['category only', '', 'Productivity', ['cafe', 'ghost']],
    ['category and query', 'ghost', 'Monitoring', []],
    ['no match', 'zzz', ALL_CATEGORIES, []],
  ]
  it.each(cases)('%s', (_label, query, category, expected) => {
    expect(filterTemplates(CATALOG, query, category).map((t) => t.id)).toEqual(
      expected,
    )
  })
})

describe('sortTemplates', () => {
  it('sorts by name', () => {
    expect(sortTemplates(CATALOG, 'name').map((t) => t.id)).toEqual([
      'cafe',
      'ghost',
      'grafana',
      'redis',
    ])
  })

  it('sorts lightest first with unknown memory last', () => {
    expect(sortTemplates(CATALOG, 'memory').map((t) => t.id)).toEqual([
      'cafe',
      'ghost',
      'grafana',
      'redis',
    ])
  })

  it('ranks name hits above description hits for relevance', () => {
    expect(
      sortTemplates(CATALOG, 'relevance', 'grafana').map((t) => t.id),
    ).toEqual(['grafana', 'redis', 'cafe', 'ghost'])
  })

  it('keeps catalog order for relevance without a query and does not mutate', () => {
    const copy = [...CATALOG]
    expect(sortTemplates(CATALOG, 'relevance').map((t) => t.id)).toEqual(
      copy.map((t) => t.id),
    )
    expect(CATALOG).toEqual(copy)
  })

  it('handles an empty list', () => {
    expect(sortTemplates([], 'memory')).toEqual([])
  })
})

describe('countCategories and memoryTier', () => {
  it('counts by size then name', () => {
    expect(countCategories(CATALOG)).toEqual([
      ['Productivity', 2],
      ['Database Tools', 1],
      ['Monitoring', 1],
    ])
  })

  it('maps memory to meter bars', () => {
    expect([128, 256, 512, 1024, 2048].map((m) => memoryTier(m * MIB))).toEqual(
      [1, 1, 2, 2, 3],
    )
  })
})
