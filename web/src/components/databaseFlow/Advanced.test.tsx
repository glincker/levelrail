import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../../test/databasesI18n'
import { resolveSubmittedNodeId } from '../../lib/createResourcePlacement'
import { AUTO_NODE_VALUE } from '../PlacementFields'
import { Advanced, LOCAL_NODE, NO_PROJECT } from './Advanced'

const nodes = vi.hoisted(() => ({ list: [] as { id: string; name: string }[] }))

vi.mock('../../queries/nodes', () => ({
  useNodeListOptional: () => ({ data: nodes.list }),
}))
vi.mock('../../queries/projects', () => ({
  useProjectListOptional: () => ({ data: [] }),
}))

function show() {
  render(
    <DatabasesI18n>
      <Advanced
        nodeId={LOCAL_NODE}
        projectId={NO_PROJECT}
        onNode={() => undefined}
        onProject={() => undefined}
      />
    </DatabasesI18n>,
  )
}

describe('database create node picker', () => {
  afterEach(cleanup)

  it('is hidden under Advanced with a single node', () => {
    nodes.list = [{ id: 'n1', name: 'node-a' }]
    show()
    expect(document.getElementById('database-node')).toBeNull()
  })

  it('shows at top level when more than one node exists', () => {
    nodes.list = [
      { id: 'n1', name: 'node-a' },
      { id: 'n2', name: 'node-b' },
    ]
    show()
    expect(document.getElementById('database-node')).not.toBeNull()
    expect(screen.getByLabelText('Node')).toBeInTheDocument()
  })
})

describe('resolveSubmittedNodeId', () => {
  it('omits the node for automatic placement', () => {
    expect(resolveSubmittedNodeId(true, AUTO_NODE_VALUE)).toBeUndefined()
  })
  it('sends a chosen node', () => {
    expect(resolveSubmittedNodeId(true, 'n2')).toBe('n2')
  })
})
