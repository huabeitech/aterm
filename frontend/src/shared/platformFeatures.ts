/** Features intentionally omitted from the first Windows release. */
export function platformFeatures(platform: string) {
  return {
    openSSHExport: platform !== 'win32',
    cloudBackup: platform !== 'win32'
  }
}
