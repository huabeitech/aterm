import { AppError } from '../errors'
import { t } from './index'
export function localizedError(error: unknown): string {
  if (error instanceof AppError) return t(`errors:${error.code}`, error.params)
  return error instanceof Error ? error.message : t('errors:unexpected')
}
