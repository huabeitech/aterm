import { effectiveAppTheme, themeDefinition, availableThemes } from './themeRegistry'
import { resolveTheme, type ResolvedTheme } from '../../../shared/contracts/resolvedTheme'
import type { ResolvedAppearance, ThemeID } from '../../../shared/contracts/themes'
import { t } from '../../../shared/i18n'
export type AppTheme = ResolvedTheme
export function getAppTheme(id: ThemeID, appearance: ResolvedAppearance): ResolvedTheme {
  return resolveTheme(themeDefinition(effectiveAppTheme(id, appearance)), appearance)
}
export function themeOptions() {
  return [
    { value: 'system', label: t('settings:system') },
    ...availableThemes().map((theme) => ({ value: theme.reference, label: theme.name }))
  ]
}
