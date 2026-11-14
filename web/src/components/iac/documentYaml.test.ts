import { describe, expect, it } from 'vitest'
import {
  parseYaml,
  readDocument,
  serializeDocument,
  valuesFromTree,
  YamlParseError,
  type FormValues,
} from './documentYaml'
import { realSchema } from './iacTestSchema'
import { schemaToFields } from './schemaToFields'

const app = schemaToFields(realSchema, 'App')

describe('documentYaml', () => {
  it('round trips values to yaml to values', () => {
    const values: FormValues = {
      'metadata.name': 'shop',
      'metadata.labels': [{ key: 'team', value: 'core', secret: false }],
      'spec.project': 'main',
      'spec.service.image': 'nginx:1.25',
      'spec.service.port': '8080',
      'spec.service.domains': ['shop.example.com', 'www.example.com'],
      'spec.service.env': [
        { key: 'MODE', value: 'prod', secret: false },
        { key: 'DB_URL', value: 'shop-db-url', secret: true },
      ],
    }
    const yaml = serializeDocument(app, values)
    expect(yaml).toContain('version: 1\nkind: App')
    expect(yaml).toContain('port: 8080')
    const doc = readDocument(yaml)
    expect(doc.kind).toBe('App')
    const back = valuesFromTree(app, doc.tree)
    expect(back.values).toEqual(values)
    expect(back.extra).toEqual({})
    expect(serializeDocument(app, back.values, back.extra)).toBe(yaml)
  })

  it('keeps fields the form cannot edit', () => {
    const src = [
      'version: 1',
      'kind: App',
      'metadata:',
      '  name: shop',
      'spec:',
      '  service:',
      '    image: nginx',
      '    volumes:',
      '      - name: data',
      '        mount: /data',
    ].join('\n')
    const doc = readDocument(src)
    const { values, extra } = valuesFromTree(app, doc.tree)
    const out = readDocument(serializeDocument(app, values, extra)).tree
    expect(out).toEqual(doc.tree)
  })

  it('never emits a raw value for a secret-like env key', () => {
    const yaml = serializeDocument(app, {
      'metadata.name': 'shop',
      'spec.service.env': [
        { key: 'API_TOKEN', value: 'hunter2-raw', secret: false },
        { key: 'DB_PASSWORD', value: 'db-pass-ref', secret: true },
      ],
    })
    expect(yaml).not.toContain('hunter2-raw')
    expect(yaml).toContain('secretRef: db-pass-ref')
  })

  it('parses flow collections, quotes and comments', () => {
    expect(
      parseYaml("a: [x, \"y z\", 3]\nb: {k: v}  # note\nc: 'it''s'\n"),
    ).toEqual({
      a: ['x', 'y z', 3],
      b: { k: 'v' },
      c: "it's",
    })
  })

  it('reports the line of a bad document', () => {
    expect(() => parseYaml('a: 1\n  b: 2\n')).toThrow(YamlParseError)
  })
})
