import { t } from '../../../../shared/i18n'
export const DIRECTORY_NAME_MAX_LENGTH = 50
export const DIRECTORY_NAME_SPECIAL_CHARACTERS = '/ \\ : * ? " < > |'

const DIRECTORY_NAME_INVALID_PATTERN = /[/\\:*?"<>|\p{C}]/u

export function validateDirectoryName(name: string) {
  const trimmed = name.trim()
  if (!trimmed) return t('hosts:directoryNameIsRequired')
  if ([...trimmed].length > DIRECTORY_NAME_MAX_LENGTH) {
    return t('hosts:directoryNameLimit', { max: DIRECTORY_NAME_MAX_LENGTH })
  }
  if (DIRECTORY_NAME_INVALID_PATTERN.test(trimmed)) {
    return t('hosts:directoryNameCharacters', { characters: DIRECTORY_NAME_SPECIAL_CHARACTERS })
  }
  return ''
}
