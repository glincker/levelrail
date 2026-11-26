// Parses the text plan returned by the explain route into a tree and
// marks the node with the highest own cost. Returns null when no line
// carries a cost, so the caller falls back to raw text.

export interface PlanNode {
  label: string
  details: string[]
  totalCost: number | null
  rows: number | null
  actualMs: number | null
  // Own cost: total minus children, floored at zero.
  selfCost: number
  hottest: boolean
  children: PlanNode[]
}

const COST = /\(cost=([\d.]+)(?:\.\.([\d.]+))?\s+rows=(\d+)/
const ACTUAL = /actual time=[\d.]+\.\.([\d.]+)/

interface Draft {
  indent: number
  node: PlanNode
}

function makeNode(line: string): PlanNode {
  const cost = COST.exec(line)
  const actual = ACTUAL.exec(line)
  const total = cost ? Number(cost[2] ?? cost[1]) : null
  return {
    label: line.replace(/\s*\((?:cost|actual)=?[^)]*\)/g, '').trim(),
    details: [],
    totalCost: total !== null && Number.isFinite(total) ? total : null,
    rows: cost ? Number(cost[3]) : null,
    actualMs: actual ? Number(actual[1]) : null,
    selfCost: 0,
    hottest: false,
    children: [],
  }
}

export function parsePlan(lines: string[]): PlanNode[] | null {
  const roots: PlanNode[] = []
  const stack: Draft[] = []
  let last: PlanNode | null = null
  let any = false

  for (const raw of lines.flatMap((l) => l.split('\n'))) {
    if (raw.trim() === '') continue
    const arrow = /^(\s*)->\s*(.*)$/.exec(raw)
    const isRoot = !arrow && stack.length === 0 && COST.test(raw)
    if (!arrow && !isRoot) {
      last?.details.push(raw.trim())
      continue
    }
    const indent = arrow ? (arrow[1] ?? '').length : 0
    const node = makeNode(arrow ? (arrow[2] ?? '') : raw)
    if (node.totalCost !== null) any = true
    while (
      stack.length > 0 &&
      (stack[stack.length - 1] as Draft).indent >= indent
    ) {
      stack.pop()
    }
    const parent = stack[stack.length - 1]
    if (parent) parent.node.children.push(node)
    else roots.push(node)
    stack.push({ indent, node })
    last = node
  }
  if (!any || roots.length === 0) return null

  const best: { node: PlanNode | null } = { node: null }
  const walk = (n: PlanNode) => {
    const kids = n.children.reduce((s, c) => s + (c.totalCost ?? 0), 0)
    n.selfCost = Math.max(0, (n.totalCost ?? 0) - kids)
    if (best.node === null || n.selfCost > best.node.selfCost) best.node = n
    n.children.forEach(walk)
  }
  roots.forEach(walk)
  if (best.node && best.node.selfCost > 0) best.node.hottest = true
  return roots
}
