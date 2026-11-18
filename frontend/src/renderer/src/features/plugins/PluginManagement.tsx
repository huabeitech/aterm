import type { PluginActivity } from '../../../../shared/contracts/pluginExecution'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { t, i18n } from '../../../../shared/i18n'
import { pluginDisplayName } from '../../../../shared/plugins/localization'
import type { PluginInfo } from '../../../../shared/contracts/plugins'
import { desktop } from '../../api/app'
import { ActionButton, SelectField } from '../../components/ui/workbench-controls'
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '../../components/ui/dialog'
import { useAppUI } from '../../components/ui/confirm-provider'

export function PluginManagement({ macAppStoreBuild }: { macAppStoreBuild: boolean }) {
  useTranslation()
  const [plugins, setPlugins] = useState<PluginInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [pending, setPending] = useState<Record<string, string>>({})
  const pendingRef = useRef(new Set<string>())
  const [pluginErrors, setPluginErrors] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [activity, setActivity] = useState<Record<string, PluginActivity[]>>({})
  const [activityState, setActivityState] = useState<Record<string, 'loading' | 'ready' | 'error'>>(
    {},
  )
  const { modal } = useAppUI()
  useEffect(() => {
    let disposed = false
    const load = () =>
      void desktop.plugins
        .list()
        .then((value) => {
          if (!disposed) {
            setPlugins(value)
            setLoading(false)
          }
        })
        .catch((error) => {
          if (!disposed) {
            setError(String(error))
            setLoading(false)
          }
        })
    const unsubscribe = desktop.plugins.onChanged(load)
    load()
    return () => {
      disposed = true
      unsubscribe()
    }
  }, [])
  async function run(scope: string, operation: string, action: () => Promise<void>) {
    if (pendingRef.current.has(scope)) return
    pendingRef.current.add(scope)
    setPending((all) => ({ ...all, [scope]: operation }))
    if (scope === 'install') setError('')
    else setPluginErrors((all) => ({ ...all, [scope]: '' }))
    try {
      await action()
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      if (scope === 'install') setError(message)
      else setPluginErrors((all) => ({ ...all, [scope]: message }))
    } finally {
      pendingRef.current.delete(scope)
      setPending((all) => {
        const next = { ...all }
        delete next[scope]
        return next
      })
    }
  }
  async function loadActivity(id: string) {
    setActivityState((all) => ({ ...all, [id]: 'loading' }))
    try {
      const rows = await desktop.plugins.activity(id)
      setActivity((all) => ({ ...all, [id]: rows }))
      setActivityState((all) => ({ ...all, [id]: 'ready' }))
    } catch {
      setActivityState((all) => ({ ...all, [id]: 'error' }))
    }
  }
  return (
    <div className="plugin-management" data-testid="plugin-management">
      <div className="plugin-management-heading">
        <p className="text-sm text-muted-foreground">{t('settings:pluginsDescription')}</p>
        {!macAppStoreBuild && (
          <ActionButton
            busy={Boolean(pending.install)}
            onClick={() => void run('install', 'install', () => desktop.plugins.install())}
          >
            {t(pending.install ? 'settings:pluginsInstalling' : 'settings:pluginsInstallArchive')}
          </ActionButton>
        )}
      </div>
      {macAppStoreBuild && (
        <p className="text-sm text-muted-foreground">{t('settings:pluginsMas')}</p>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {loading && (
        <div
          className="plugin-management-empty"
          role="status"
          aria-label={t('settings:pluginsLoading')}
        >
          {t('settings:pluginsLoading')}
        </div>
      )}
      {!loading && !error && !plugins.length && (
        <p className="text-sm text-muted-foreground">{t('settings:pluginsEmpty')}</p>
      )}
      {plugins.map((plugin) => (
        <section
          key={plugin.manifest.id}
          className="plugin-package"
          aria-label={pluginDisplayName(
            plugin.manifest,
            plugin.localizations,
            i18n.resolvedLanguage ?? i18n.language,
          )}
        >
          <div className="plugin-package-heading">
            <div className="min-w-0">
              <h2 className="plugin-package-name">
                {pluginDisplayName(
                  plugin.manifest,
                  plugin.localizations,
                  i18n.resolvedLanguage ?? i18n.language,
                )}
              </h2>
              <p
                className="plugin-package-status"
                data-state={!plugin.enabled ? 'disabled' : plugin.state}
                role="status"
              >
                <span className="plugin-state-dot" aria-hidden="true" />
                <span>
                  {!plugin.enabled
                    ? t('settings:pluginsDisabled')
                    : plugin.state === 'active'
                      ? t('settings:pluginsActive')
                      : plugin.state === 'failed'
                        ? t('settings:pluginsFailed')
                        : t('settings:pluginsInactive')}
                </span>
              </p>
              <p className="plugin-package-meta">
                {plugin.manifest.id} · {plugin.manifest.version} ·{' '}
                {plugin.builtIn ? t('settings:pluginsBuiltIn') : t('settings:pluginsLocal')}
              </p>
            </div>
            <ActionButton
              busy={pending[plugin.manifest.id] === 'toggle'}
              disabled={Boolean(pending[plugin.manifest.id])}
              onClick={() =>
                void run(plugin.manifest.id, 'toggle', () =>
                  desktop.plugins.configure(plugin.manifest.id, !plugin.enabled, plugin.interval),
                )
              }
            >
              {pending[plugin.manifest.id] === 'toggle'
                ? t(plugin.enabled ? 'settings:pluginsDisabling' : 'settings:pluginsEnabling')
                : plugin.enabled
                  ? t('settings:pluginsDisable')
                  : t('settings:pluginsEnable')}
            </ActionButton>
          </div>
          {(pluginErrors[plugin.manifest.id] || plugin.error) && (
            <p className="plugin-package-error" role="alert">
              {pluginErrors[plugin.manifest.id] || plugin.error}
            </p>
          )}
          {pending[plugin.manifest.id] && pending[plugin.manifest.id] !== 'toggle' && (
            <p className="plugin-package-feedback" role="status">
              {t(
                pending[plugin.manifest.id] === 'rollback'
                  ? 'settings:pluginsRollingBack'
                  : pending[plugin.manifest.id] === 'uninstall'
                    ? 'settings:pluginsUninstalling'
                    : pending[plugin.manifest.id]?.startsWith('revoke:')
                      ? 'settings:pluginsRevoking'
                      : 'settings:pluginsSaving',
              )}
            </p>
          )}
          {plugin.manifest.runtime !== 'native' && (
            <dl className="plugin-settings-rows">
              <div className="settings-row">
                <dt className="settings-row-label">{t('settings:pluginsInterval')}</dt>
                <dd className="settings-row-control">
                  <SelectField
                    aria-label={t('settings:pluginsInterval')}
                    value={String(plugin.interval)}
                    disabled={Boolean(pending[plugin.manifest.id])}
                    options={[5, 10, 30].map((n) => ({
                      value: String(n),
                      label: t('settings:pluginsSeconds', { count: n }),
                    }))}
                    onValueChange={(value) =>
                      void run(plugin.manifest.id, 'configure', () =>
                        desktop.plugins.configure(
                          plugin.manifest.id,
                          plugin.enabled,
                          Number(value),
                        ),
                      )
                    }
                  />
                </dd>
              </div>
            </dl>
          )}
          <footer className="plugin-maintenance">
            {plugin.state === 'failed' && (
              <ActionButton
                disabled={Boolean(pending[plugin.manifest.id])}
                onClick={() =>
                  void run(plugin.manifest.id, 'retry', () =>
                    desktop.plugins.configure(plugin.manifest.id, true, plugin.interval),
                  )
                }
              >
                {t('settings:pluginsRetry')}
              </ActionButton>
            )}
            <Dialog>
              <DialogTrigger asChild>
                <ActionButton className="plugin-maintenance-button">
                  {t('settings:pluginsAuthorization')}
                </ActionButton>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>{t('settings:pluginsAuthorization')}</DialogTitle>
                  <DialogDescription>
                    {pluginDisplayName(
                      plugin.manifest,
                      plugin.localizations,
                      i18n.resolvedLanguage ?? i18n.language,
                    )}
                  </DialogDescription>
                </DialogHeader>
                {plugin.manifest.execution ? (
                  <p className="plugin-permission-note">
                    {t('settings:pluginsPermissionExecution', {
                      programs: plugin.manifest.execution.programs.join(', '),
                      targets: plugin.manifest.execution.targets.join(', '),
                      terminal: plugin.manifest.permissions.includes('terminal.create')
                        ? t('settings:pluginsExecutionTerminal')
                        : '',
                    })}
                  </p>
                ) : null}
                {plugin.manifest.runtime === 'native' && (
                  <p className="plugin-permission-note">
                    {t('native:nativePlugin.installMessage')}
                  </p>
                )}
                <div className="plugin-grants">
                  <h3>{t('settings:pluginsAuthorizedHosts')}</h3>
                  {!plugin.grants.length && (
                    <p className="text-xs text-muted-foreground">{t('settings:pluginsNoHosts')}</p>
                  )}
                  {plugin.grants.map((grant) => (
                    <div key={grant.id} className="plugin-grant-row">
                      <span className="truncate" title={grant.label}>
                        {grant.label}
                      </span>
                      <ActionButton
                        disabled={Boolean(pending[plugin.manifest.id])}
                        onClick={() =>
                          void run(plugin.manifest.id, `revoke:${grant.id}`, () =>
                            desktop.plugins.revoke(plugin.manifest.id, grant.id),
                          )
                        }
                      >
                        {t(
                          pending[plugin.manifest.id] === `revoke:${grant.id}`
                            ? 'settings:pluginsRevoking'
                            : 'settings:pluginsRevoke',
                        )}
                      </ActionButton>
                    </div>
                  ))}
                </div>
                {pluginErrors[plugin.manifest.id] && (
                  <p role="alert" className="text-sm text-destructive">
                    {pluginErrors[plugin.manifest.id]}
                  </p>
                )}
              </DialogContent>
            </Dialog>
            {plugin.canRollback && (
              <ActionButton
                className="plugin-maintenance-button"
                disabled={Boolean(pending[plugin.manifest.id])}
                onClick={() =>
                  void run(plugin.manifest.id, 'rollback', () =>
                    desktop.plugins.rollback(plugin.manifest.id),
                  )
                }
              >
                {t(
                  pending[plugin.manifest.id] === 'rollback'
                    ? 'settings:pluginsRollingBack'
                    : 'settings:pluginsRollback',
                )}
              </ActionButton>
            )}
            {plugin.manifest.execution && (
              <Dialog
                onOpenChange={(open) => {
                  if (open) void loadActivity(plugin.manifest.id)
                }}
              >
                <DialogTrigger asChild>
                  <ActionButton className="plugin-maintenance-button">
                    {t('settings:pluginsExecutionActivity')}
                  </ActionButton>
                </DialogTrigger>
                <DialogContent className="plugin-activity-dialog sm:max-w-2xl">
                  <DialogHeader>
                    <DialogTitle>{t('settings:pluginsExecutionActivity')}</DialogTitle>
                    <p className="plugin-activity-plugin-name">
                      {pluginDisplayName(
                        plugin.manifest,
                        plugin.localizations,
                        i18n.resolvedLanguage ?? i18n.language,
                      )}
                    </p>
                    <DialogDescription>
                      {t('settings:pluginsExecutionActivityHelp')}
                    </DialogDescription>
                  </DialogHeader>
                  <div
                    className="plugin-activity-body"
                    role="log"
                    aria-label={t('settings:pluginsExecutionActivity')}
                  >
                    {activityState[plugin.manifest.id] === 'loading' ? (
                      <p role="status">{t('settings:pluginsActivityLoading')}</p>
                    ) : activityState[plugin.manifest.id] === 'error' ? (
                      <div className="plugin-activity-failure">
                        <p role="alert">{t('settings:pluginsActivityError')}</p>
                        <ActionButton onClick={() => void loadActivity(plugin.manifest.id)}>
                          {t('settings:pluginsRetry')}
                        </ActionButton>
                      </div>
                    ) : activityState[plugin.manifest.id] === 'ready' &&
                      activity[plugin.manifest.id]?.length ? (
                      <ul className="plugin-activity-list">
                        {activity[plugin.manifest.id].map((row, index) => (
                          <li key={index} className="plugin-activity-row">
                            <time dateTime={row.time}>
                              {new Date(row.time).toLocaleString(i18n.language)}
                            </time>
                            <span className="plugin-activity-outcome" data-outcome={row.outcome}>
                              {t(`settings:pluginsExecutionOutcome${row.outcome}`)}
                            </span>
                            <code>
                              {row.executable} · {row.capability}
                            </code>
                            <code title={row.target}>{row.target}</code>
                          </li>
                        ))}
                      </ul>
                    ) : activityState[plugin.manifest.id] === 'ready' ? (
                      <p>{t('settings:pluginsExecutionActivityEmpty')}</p>
                    ) : null}
                  </div>
                </DialogContent>
              </Dialog>
            )}
            {!plugin.builtIn && (
              <ActionButton
                className="plugin-maintenance-button plugin-uninstall"
                disabled={Boolean(pending[plugin.manifest.id])}
                onClick={() =>
                  modal.confirm({
                    title: t('settings:pluginsUninstall'),
                    content: pluginDisplayName(
                      plugin.manifest,
                      plugin.localizations,
                      i18n.resolvedLanguage ?? i18n.language,
                    ),
                    okButtonProps: { danger: true },
                    onOk: () =>
                      run(plugin.manifest.id, 'uninstall', () =>
                        desktop.plugins.uninstall(plugin.manifest.id),
                      ),
                  })
                }
              >
                {t(
                  pending[plugin.manifest.id] === 'uninstall'
                    ? 'settings:pluginsUninstalling'
                    : 'settings:pluginsUninstall',
                )}
              </ActionButton>
            )}
          </footer>
        </section>
      ))}
    </div>
  )
}
