import { entries, galleryParams } from '../.vitepress/templates.mts'

export default {
  paths() {
    return entries.map((e) => ({ params: galleryParams(e) }))
  },
}
