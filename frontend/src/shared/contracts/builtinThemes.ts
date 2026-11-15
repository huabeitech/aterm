import data from './builtin-themes.json' with { type: 'json' }
import { validateThemePackage } from './themePackages'
/** Bundled packages use the same contract and validation, with a reserved namespace. */
export const builtinThemePackage = validateThemePackage(data, true)
