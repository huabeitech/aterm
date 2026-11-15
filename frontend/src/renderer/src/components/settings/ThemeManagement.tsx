import { useState } from 'react'
import { t } from '../../../../shared/i18n'
import type { Preferences } from '../../../../shared/contracts/preferences'
import type { ThemeID } from '../../../../shared/contracts/themes'
import { desktop } from '../../api/app'
import {
  availableThemes,
  themeCatalog,
  useThemeRevision,
  effectiveAppTheme
} from '../../app/themeRegistry'
import { ActionButton, SelectField, WorkbenchDialog } from '../ui/workbench-controls'

export function ThemeManagement({
  settings,
  onChange
}: {
  settings: Preferences
  onChange: (next: Preferences) => Promise<void>
}) {
  useThemeRevision()
  const themes = availableThemes()
  const catalog = themeCatalog()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [removing, setRemoving] = useState<(typeof themes)[number]>()
  async function run(action: () => Promise<unknown>) {
    setBusy(true)
    setError('')
    try {
      await action()
    } catch (error) {
      setError(error instanceof Error ? error.message : String(error))
    } finally {
      setBusy(false)
    }
  }
  const appearance = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  const configured =
    settings.theme === 'system'
      ? appearance === 'light'
        ? (settings.lightTheme ?? 'light')
        : (settings.darkTheme ?? 'dark')
      : settings.theme
  const active = themes.some((theme) => theme.reference === configured) ? configured : appearance
  async function activate(reference: string) {
    await onChange({ ...settings, theme: reference as ThemeID })
  }
  const missing = [settings.theme, settings.lightTheme, settings.darkTheme].filter(
    (id) => id && id !== 'system' && !themes.some((theme) => theme.reference === id)
  )
  const referenced =
    removing &&
    [settings.theme, settings.lightTheme, settings.darkTheme].includes(
      removing.reference as ThemeID
    )
  return (
    <div className="theme-management" aria-label={t('settings:manageThemes')}>
      <div className="theme-management-actions">
        <ActionButton disabled={busy} onClick={() => void run(() => desktop.themes.import())}>
          {t('settings:importTheme')}
        </ActionButton>
        <ActionButton
          disabled={busy}
          onClick={() =>
            void run(() => desktop.themes.duplicate(effectiveAppTheme(settings.theme, appearance)))
          }
        >
          {t('settings:duplicateTheme')}
        </ActionButton>
      </div>
      <label className="theme-management-row">
        <span>{t('settings:themeMode')}</span>
        <SelectField
          aria-label={t('settings:themeMode')}
          disabled={busy}
          value={settings.theme === 'system' ? 'system' : 'fixed'}
          options={[
            { value: 'system', label: t('settings:system') },
            { value: 'fixed', label: t('settings:fixedTheme') }
          ]}
          onValueChange={(mode) =>
            void run(async () => {
              await onChange({
                ...settings,
                theme: mode === 'system' ? 'system' : (active as ThemeID)
              })
            })
          }
        />
      </label>
      {settings.theme === 'system' &&
        (['light', 'dark'] as const).map((mode) => (
          <label className="theme-management-row" key={mode}>
            <span>
              {t(mode === 'light' ? 'settings:systemLightTheme' : 'settings:systemDarkTheme')}
            </span>
            <SelectField
              aria-label={t(
                mode === 'light' ? 'settings:systemLightTheme' : 'settings:systemDarkTheme'
              )}
              disabled={busy}
              value={settings[mode === 'light' ? 'lightTheme' : 'darkTheme'] ?? mode}
              options={themes
                .filter((theme) => theme.appearance === mode)
                .map((theme) => ({ value: theme.reference, label: theme.name }))}
              onValueChange={(id) =>
                void run(() =>
                  onChange({ ...settings, [mode === 'light' ? 'lightTheme' : 'darkTheme']: id })
                )
              }
            />
          </label>
        ))}
      <ul className="theme-list" aria-label={t('settings:themeList')}>
        {themes.map((theme) => (
          <li
            className="theme-package"
            key={theme.reference}
            data-theme-reference={theme.reference}
            data-active={active === theme.reference}
          >
            <div className="theme-info">
              <div className="theme-heading">
                <strong>{theme.name}</strong>
                <span className="theme-meta">
                  {t(theme.builtIn ? 'settings:builtinTheme' : 'settings:customTheme')} ·{' '}
                  {t(theme.appearance === 'light' ? 'settings:light' : 'settings:dark')}
                </span>
                <div className="theme-status">
                  {active === theme.reference && <span>{t('settings:themeInUse')}</span>}
                  {settings.theme === 'system' &&
                    (settings.lightTheme ?? 'light') === theme.reference && (
                      <span>{t('settings:systemLightTheme')}</span>
                    )}
                  {settings.theme === 'system' &&
                    (settings.darkTheme ?? 'dark') === theme.reference && (
                      <span>{t('settings:systemDarkTheme')}</span>
                    )}
                </div>
              </div>
              {!theme.builtIn && (
                <span
                  className="theme-meta theme-author"
                  title={`${theme.packageName} · ${theme.version} · ${theme.author}`}
                >
                  {theme.packageName} · {theme.version} · {theme.author}
                </span>
              )}
            </div>
            <div className="theme-management-actions">
              <ActionButton
                disabled={busy || settings.theme === theme.reference}
                onClick={() => void run(() => activate(theme.reference))}
              >
                {t('settings:activateTheme')}
              </ActionButton>
              {!theme.builtIn && (
                <>
                  <ActionButton
                    disabled={busy}
                    onClick={() => void run(() => desktop.themes.export(theme.packageID))}
                  >
                    {t('settings:exportTheme')}
                  </ActionButton>
                  <ActionButton
                    className="theme-remove"
                    disabled={busy}
                    onClick={() => setRemoving(theme)}
                  >
                    {t('settings:removeTheme')}
                  </ActionButton>
                </>
              )}
            </div>
          </li>
        ))}
      </ul>
      {missing.length > 0 && (
        <p role="status">
          {t('settings:missingTheme')}: {[...new Set(missing)].join(', ')}
        </p>
      )}
      {catalog.warnings.map((warning) => (
        <p role="status" key={warning}>
          {warning}
        </p>
      ))}
      {error && <p role="alert">{error}</p>}
      <WorkbenchDialog
        open={!!removing}
        title={t('settings:removeTheme')}
        onClose={() => setRemoving(undefined)}
        busy={busy}
        submitLabel={t('settings:removeTheme')}
        onSubmit={() =>
          void run(async () => {
            if (!removing) return
            await desktop.themes.remove(removing.reference)
            setRemoving(undefined)
          })
        }
      >
        <p>
          {t(
            referenced
              ? 'settings:removeActiveThemeDescription'
              : 'settings:removeThemeDescription',
            { name: removing?.name ?? '' }
          )}
        </p>
      </WorkbenchDialog>
    </div>
  )
}
