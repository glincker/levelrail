// Drop deploy: upload a zip of a static site to POST /api/v1/apps/{name}/uploads, then build it as build.type static.

import { ApiError } from '../lib/apiError'
import { PENDING_IMAGE_TAG } from '../lib/newApp'
import { createApp, deleteApp } from './apps'
import { triggerBuild } from './builds'

export const STATIC_SITE_PORT = 80

export interface SiteUpload {
  hash: string
  file_count: number
  has_index?: boolean
  site_root?: string
}

function parseErrorBody(text: string, fallback: string): string {
  try {
    const body = JSON.parse(text) as { error?: string; message?: string }
    return body.error ?? body.message ?? fallback
  } catch {
    return fallback
  }
}

/** XHR rather than fetch because only XHR reports upload progress. */
export function uploadSiteZip(
  appName: string,
  body: Blob,
  onProgress: (fraction: number) => void,
): Promise<SiteUpload> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', `/api/v1/apps/${encodeURIComponent(appName)}/uploads`)
    xhr.setRequestHeader('Content-Type', 'application/zip')
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && e.total > 0) onProgress(e.loaded / e.total)
    }
    xhr.onerror = () => reject(new ApiError(0, 'network'))
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(JSON.parse(xhr.responseText) as SiteUpload)
        return
      }
      reject(
        new ApiError(
          xhr.status,
          parseErrorBody(xhr.responseText, `upload failed: ${xhr.status}`),
        ),
      )
    }
    xhr.send(body)
  })
}

export type DropStage = 'creating' | 'uploading' | 'publishing'

export interface PublishDroppedSiteInput {
  appName: string
  /** True when the app does not exist yet and should be created first. */
  create: boolean
  zip: Blob
  onStage: (stage: DropStage) => void
  onProgress: (fraction: number) => void
}

export interface PublishDroppedSiteResult {
  deployId: string
  upload: SiteUpload
}

/** Creates the app if asked, uploads the zip and starts the static publish. A site created here is removed again if the upload fails. */
export async function publishDroppedSite(
  input: PublishDroppedSiteInput,
): Promise<PublishDroppedSiteResult> {
  const { appName } = input
  if (input.create) {
    input.onStage('creating')
    await createApp({
      name: appName,
      image: `${appName}${PENDING_IMAGE_TAG}`,
      port: STATIC_SITE_PORT,
    })
  }
  try {
    input.onStage('uploading')
    const upload = await uploadSiteZip(appName, input.zip, input.onProgress)
    input.onStage('publishing')
    const built = await triggerBuild(appName, {
      repoUrl: '',
      ref: '',
      buildType: 'static',
      uploadHash: upload.hash,
    })
    return { deployId: built.id ?? '', upload }
  } catch (err) {
    if (input.create) {
      await deleteApp(appName).catch(() => undefined)
    }
    throw err
  }
}
