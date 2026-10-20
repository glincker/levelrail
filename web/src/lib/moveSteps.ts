export function moveSteps(appName: string, volumeNames: string[]): string[] {
  return [
    `Stop ${appName} on its current node`,
    ...volumeNames.map((v) => `Copy volume ${v} to the destination`),
    'Point the app at the destination node',
    `Start ${appName} there`,
  ]
}
