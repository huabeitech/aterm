import type { FileTransfer } from '../../../../shared/contracts/files'
import { i18n, t } from '../../../../shared/i18n'
export function fileSize(size: number) {
  if (size < 1024) return `${new Intl.NumberFormat(i18n.language).format(size)} B`
  const unit = Math.min(3, Math.floor(Math.log(size) / Math.log(1024)))
  return `${new Intl.NumberFormat(i18n.language, { maximumFractionDigits: size / 1024 ** unit < 10 ? 1 : 0 }).format(size / 1024 ** unit)} ${['B', 'KB', 'MB', 'GB'][unit]}`
}

export function uploadTimeRemaining({ total, transferred, bytesPerSecond }: FileTransfer) {
  if (total <= 0 || !bytesPerSecond || bytesPerSecond <= 0) {
    return t('files:transferEstimatingTime')
  }

  const seconds = Math.ceil(Math.max(0, total - transferred) / bytesPerSecond)
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const parts = [String(minutes).padStart(2, '0'), String(seconds % 60).padStart(2, '0')]
  if (hours > 0) parts.unshift(String(hours))
  return t('files:transferTimeRemaining', { duration: parts.join(':') })
}
