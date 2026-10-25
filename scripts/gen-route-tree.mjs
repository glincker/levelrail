// Regenerates web/src/routeTree.gen.ts without a full vite build (seconds, not
// a minute). Same generator and options as the tanstackRouter plugin in
// web/vite.config.ts. Run from anywhere: node scripts/gen-route-tree.mjs
import { createRequire } from 'node:module'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const web = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'web')
const require = createRequire(path.join(web, 'package.json'))
const { Generator, getConfig } = await import(require.resolve('@tanstack/router-generator'))

const config = getConfig({ target: 'react', autoCodeSplitting: true }, web)
await new Generator({ config, root: web }).run()
