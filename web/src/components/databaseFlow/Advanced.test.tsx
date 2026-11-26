import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../../test/databasesI18n'
import { resolveSubmittedNodeId } from '../../lib/createResourcePlacement'
import { AUTO_NODE_VALUE } from '../PlacementFields'
import { Advanced, NO_PROJECT } from './Advanced'

vi.mock('../../queries/projects', () => ({
  useProjectListOptional: () => ({ data: [] }),
}))

describe('database create advanced options', () => {
  afterEach(cleanup)

  it('keeps the project picker folded and has no node picker', () => {
    render(
      <DatabasesI18n>
        <Advanced projectId={NO_PROJECT} onProject={() => undefined} />
      </DatabasesI18n>,
    )
    expect(screen.getByText('Advanced options')).toBeInTheDocument()
    expect(document.getElementById('database-node')).toBeNull()
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
