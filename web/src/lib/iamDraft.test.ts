import { describe, expect, it } from 'vitest'
import {
  documentFromJson,
  documentToStatements,
  draftToDocument,
  issuesFor,
  resourceToSelection,
  selectionToResources,
} from './iamDraft'
import type { PolicyDraft } from './iamDraft'

describe('iamDraft', () => {
  it('compiles selections to the resource strings the evaluator reads', () => {
    expect(selectionToResources({ kind: 'all' })).toEqual(['*'])
    expect(selectionToResources({ kind: 'app', name: 'web' })).toEqual([
      'app:web',
    ])
    expect(selectionToResources({ kind: 'database', name: 'main' })).toEqual([
      'database:main',
    ])
    expect(
      selectionToResources({ kind: 'environment_kind', value: 'production' }),
    ).toEqual(['environment-kind:production'])
    expect(
      selectionToResources({
        kind: 'project',
        id: 'p1',
        environmentIds: ['e1', 'e2'],
      }),
    ).toEqual(['environment:e1', 'environment:e2'])
  })

  it('round trips a document through the draft model', () => {
    const doc = {
      Statement: [
        {
          Effect: 'Allow' as const,
          Action: ['read', 'write'],
          Resource: ['app:web', 'environment-kind:dev', 'app:prod*'],
        },
        { Effect: 'Deny' as const, Action: ['*'], Resource: ['*'] },
      ],
    }
    const draft: PolicyDraft = {
      name: 'x',
      description: '',
      statements: documentToStatements(doc),
    }
    expect(draftToDocument(draft)).toEqual(doc)
    expect(resourceToSelection('app:prod*')).toEqual({
      kind: 'pattern',
      value: 'app:prod*',
    })
  })

  it('collapses a wildcard action to a single entry and de-duplicates resources', () => {
    const doc = draftToDocument({
      name: '',
      description: '',
      statements: [
        {
          effect: 'Deny',
          actions: ['read', '*'],
          resources: [
            { kind: 'app', name: 'a' },
            { kind: 'app', name: 'a' },
          ],
        },
      ],
    })
    expect(doc.Statement[0]).toEqual({
      Effect: 'Deny',
      Action: ['*'],
      Resource: ['app:a'],
    })
  })

  it('reports a bad JSON document by reason', () => {
    expect(documentFromJson('{')).toHaveProperty('error')
    expect(documentFromJson('{"Statement":3}')).toEqual({ error: 'Statement' })
    expect(documentFromJson('{"Statement":[]}')).toEqual({
      doc: { Statement: [] },
    })
  })

  it('selects issues for one statement field', () => {
    const issues = [
      { path: 'Statement[0].Action[1]' },
      { path: 'Statement[1].Action' },
      { path: 'Statement[1].Resource[0]' },
    ]
    expect(issuesFor(issues, 1, 'Action')).toEqual([
      { path: 'Statement[1].Action' },
    ])
    expect(issuesFor(issues, 0, 'Action')).toHaveLength(1)
  })
})
