export type MockStatus = 'healthy' | 'running' | 'deploying' | 'failed' | 'idle'
export type MockTone = 'blue' | 'green' | 'purple' | 'orange'
export type MockIconTile = 'web' | 'cache' | 'api' | 'docs' | 'worker'

export interface MockApp {
  id: string
  name: string
  status: MockStatus
  tile: MockIconTile
  domain?: string
  traffic: number[]
  rpm: string
  p95: string
  errors: string
  deployed: string
}

export interface MockDeploy {
  id: string
  status: 'live' | 'superseded'
  sha: string
  message: string
  image: string
  trigger: string
  when: string
  duration: string
}

export interface MockCol {
  key: string
  label: string
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'grow'
  align?: 'end'
}

export interface MockNavItem {
  id: string
  label: string
  icon: string
  children?: MockNavItem[]
}

export interface MockLog {
  time: string
  level: 'info' | 'debug'
  text: string
}

export interface MockChart {
  id: string
  title: string
  tone: MockTone
  value: string
  yTicks: string[]
  yMin: number
  yMax: number
  series: { name: string; tone: MockTone; values: number[]; area?: boolean }[]
}

export const brand = { name: 'Levelrail', sub: 'Control plane', user: 'dev' }
export const hostUrl = 'levelrail.example.com'
export const activeApp = 'marketing-site'

export const apps: MockApp[] = [
  {
    id: 'marketing-site', name: 'marketing-site', status: 'running', tile: 'web',
    domain: 'marketing-site.example.com',
    traffic: [22, 30, 26, 38, 34, 42, 39, 48, 44, 52, 47, 58],
    rpm: '412 rpm', p95: '38 ms', errors: '0.0%', deployed: '2m ago',
  },
  {
    id: 'api', name: 'api', status: 'running', tile: 'api',
    domain: 'api.example.com',
    traffic: [40, 44, 41, 50, 47, 53, 56, 52, 60, 57, 62, 66],
    rpm: '1.2k rpm', p95: '64 ms', errors: '0.0%', deployed: '18m ago',
  },
  {
    id: 'edge-cache', name: 'edge-cache', status: 'running', tile: 'cache',
    domain: 'cdn.example.com',
    traffic: [30, 32, 35, 33, 38, 36, 41, 39, 44, 42, 46, 45],
    rpm: '2.8k rpm', p95: '9 ms', errors: '0.0%', deployed: '1h ago',
  },
  {
    id: 'docs-site', name: 'docs-site', status: 'running', tile: 'docs',
    domain: 'docs.example.com',
    traffic: [12, 14, 13, 16, 15, 18, 17, 20, 19, 22, 21, 24],
    rpm: '96 rpm', p95: '27 ms', errors: '0.0%', deployed: '3h ago',
  },
]

export const appFilters = [
  { label: 'Running', count: 4, tone: 'ok' },
  { label: 'Deploying', count: 0, tone: 'warn' },
  { label: 'Failing', count: 0, tone: 'bad' },
  { label: 'Stopped', count: 0, tone: 'muted' },
] as const

export const globalNav: { section: string; items: MockNavItem[] }[] = [
  { section: 'Home', items: [
    { id: 'dashboard', label: 'Dashboard', icon: 'dashboard' },
    { id: 'status', label: 'Status', icon: 'status' },
  ] },
  { section: 'Apps', items: [
    { id: 'apps', label: 'Apps', icon: 'apps' },
    { id: 'deployments', label: 'Deployments', icon: 'deployments' },
    { id: 'projects', label: 'Projects', icon: 'projects' },
  ] },
  { section: 'Data', items: [
    { id: 'databases', label: 'Databases', icon: 'databases' },
    { id: 'backups', label: 'Backups', icon: 'backups' },
  ] },
  { section: 'Infrastructure', items: [
    { id: 'nodes', label: 'Nodes', icon: 'nodes' },
    { id: 'network', label: 'Network', icon: 'network' },
    { id: 'domains', label: 'Domains', icon: 'domains' },
    { id: 'loadbalancers', label: 'Load balancers', icon: 'loadbalancers' },
    { id: 'registry', label: 'Container registry', icon: 'registry' },
    { id: 'credentials', label: 'Registry credentials', icon: 'credentials' },
  ] },
]

export const appNav: MockNavItem[] = [
  { id: 'overview', label: 'Overview', icon: 'overview' },
  { id: 'deployments', label: 'Deployments', icon: 'deployments', children: [
    { id: 'deploys', label: 'Deploys', icon: 'deploys' },
    { id: 'source', label: 'Source', icon: 'source' },
  ] },
  { id: 'traffic', label: 'Traffic', icon: 'traffic' },
  { id: 'config', label: 'Config', icon: 'config' },
  { id: 'observe', label: 'Observe', icon: 'observe', children: [
    { id: 'metrics', label: 'Metrics', icon: 'metrics' },
    { id: 'logs', label: 'Logs', icon: 'logs' },
    { id: 'alerts', label: 'Alerts', icon: 'alerts' },
  ] },
  { id: 'exec', label: 'Exec', icon: 'exec' },
]

export const timeRanges = ['Last hour', 'Last 6 hours', 'Last 24 hours', 'Last 7 days']

const wave = (base: number, amp: number, n = 40, phase = 0): number[] =>
  Array.from({ length: n }, (_, i) =>
    +(base + amp * Math.sin(i / 4 + phase) + amp * 0.5 * Math.sin(i / 1.7 + phase)).toFixed(2))

export const xTicks = ['07:00 PM', '07:15 PM', '07:30 PM', '07:45 PM', '08:00 PM']

export const charts: MockChart[] = [
  {
    id: 'cpu', title: 'CPU', tone: 'blue', value: '3.2%',
    yTicks: ['8%', '6%', '4%', '2%', '0%'], yMin: 0, yMax: 8,
    series: [{ name: 'Usage', tone: 'blue', values: wave(3.4, 1.1), area: true }],
  },
  {
    id: 'memory', title: 'Memory', tone: 'blue', value: '186 MiB',
    yTicks: ['512', '384', '256', '128', '0'], yMin: 0, yMax: 512,
    series: [
      { name: 'Limit', tone: 'orange', values: Array.from({ length: 40 }, () => 480) },
      { name: 'Usage', tone: 'blue', values: wave(186, 14, 40, 1), area: true },
    ],
  },
  {
    id: 'network', title: 'Network I/O', tone: 'green', value: '42 KiB/s',
    yTicks: ['64', '48', '32', '16', '0'], yMin: 0, yMax: 64,
    series: [
      { name: 'Received', tone: 'green', values: wave(40, 9, 40, 2), area: true },
      { name: 'Sent', tone: 'purple', values: wave(22, 6, 40, 3) },
    ],
  },
  {
    id: 'disk', title: 'Disk I/O', tone: 'green', value: '1.4 MiB/s',
    yTicks: ['4', '3', '2', '1', '0'], yMin: 0, yMax: 4,
    series: [
      { name: 'Read', tone: 'green', values: wave(1.5, 0.6, 40, 4), area: true },
      { name: 'Write', tone: 'purple', values: wave(0.8, 0.3, 40, 5) },
    ],
  },
]

export const deployMarkers = [0.2, 0.46, 0.7, 0.92]

const imageBase = 'registry.example.com/marketing-site'

export const deploys: MockDeploy[] = [
  {
    id: 'd-0412', status: 'live', sha: '7f3a9c2', message: 'Update pricing page copy',
    image: `${imageBase}:7f3a9c2`, trigger: 'git push main', when: '2m ago', duration: '41s',
  },
  {
    id: 'd-0411', status: 'superseded', sha: '4be1d08', message: 'Add changelog feed',
    image: `${imageBase}:4be1d08`, trigger: 'git push main', when: '3h ago', duration: '38s',
  },
  {
    id: 'd-0410', status: 'superseded', sha: '91cc07a', message: 'Fix hero image sizing',
    image: `${imageBase}:91cc07a`, trigger: 'git push main', when: 'Yesterday', duration: '44s',
  },
  {
    id: 'd-0409', status: 'superseded', sha: 'c20de5f', message: 'Bump base image to node 22',
    image: `${imageBase}:c20de5f`, trigger: 'git push main', when: '2 days ago', duration: '52s',
  },
]

export const rollbackDeploy: MockDeploy = {
  id: 'd-0413', status: 'live', sha: '4be1d08', message: 'Rollback to 4be1d08',
  image: `${imageBase}:4be1d08`, trigger: 'one-click rollback', when: 'just now', duration: '6s',
}

export const rollbackTargetIndex = 1

const paths = ['/', '/pricing', '/docs/getting-started', '/assets/app.css', '/assets/app.js', '/api/health', '/blog']

export const logs: MockLog[] = Array.from({ length: 26 }, (_, i): MockLog => {
  const time = `20:03:${String((14 + Math.floor(i * 2.3)) % 60).padStart(2, '0')}`
  if (i === 9) return { time, level: 'info', text: 'deploy 7f3a9c2 passed readiness check, traffic switched' }
  if (i === 20) return { time, level: 'debug', text: 'cache warm: 128 routes prerendered in 212ms' }
  const ms = 4 + ((i * 7) % 23)
  return {
    time,
    level: 'info',
    text: `172.18.0.${2 + (i % 3)} GET ${paths[i % paths.length]} HTTP/1.1 200 ${(896 + i * 37) % 4096} ${ms}ms`,
  }
})
