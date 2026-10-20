// Starter kit stack lines, in deploy order. The list API carries no service
// breakdown, so these mirror internal/catalog/templates_starterkits.go.
export const STARTER_STACKS: Record<
  string,
  { labels: readonly string[]; services: number }
> = {
  'node-postgres-starter': { labels: ['Node.js', 'PostgreSQL'], services: 2 },
  'redis-cache-starter': { labels: ['Node.js', 'Redis'], services: 2 },
  'nextjs-postgres-starter': { labels: ['Next.js', 'PostgreSQL'], services: 2 },
  'static-site-api-starter': { labels: ['Nginx', 'Node.js'], services: 2 },
  'fastapi-postgres-starter': {
    labels: ['FastAPI', 'PostgreSQL'],
    services: 2,
  },
  'worker-redis-queue-starter': { labels: ['Node.js', 'Redis'], services: 2 },
  'nginx-multi-backend-starter': {
    labels: ['Nginx', 'Node.js'],
    services: 3,
  },
}
