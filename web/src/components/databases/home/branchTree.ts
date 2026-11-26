import type { BranchResource } from '../../../queries/databaseBranches'

export interface TreeRow {
  branch: BranchResource
  depth: number
}

// Flattens parent and child links into display order, children under their parent.
export function orderTree(branches: BranchResource[]): TreeRow[] {
  const names = new Set(branches.map((b) => b.name))
  const out: TreeRow[] = []
  const walk = (parent: string, depth: number) => {
    for (const b of branches.filter((x) => x.parent === parent)) {
      out.push({ branch: b, depth })
      walk(b.name, depth + 1)
    }
  }
  walk('', 0)
  for (const b of branches) {
    if (b.parent !== '' && !names.has(b.parent)) walk(b.parent, 0)
  }
  return out
}
