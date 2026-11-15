import { i18n } from '../../../../shared/i18n'
export function fileSize(size: number) {
  if (size < 1024) return `${new Intl.NumberFormat(i18n.language).format(size)} B`
  const unit = Math.min(3, Math.floor(Math.log(size) / Math.log(1024)))
  return `${new Intl.NumberFormat(i18n.language, { maximumFractionDigits: size / 1024 ** unit < 10 ? 1 : 0 }).format(size / 1024 ** unit)} ${['B', 'KB', 'MB', 'GB'][unit]}`
}
