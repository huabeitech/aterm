import { SettingsGroup, SettingRow } from './SettingsGroup'
import { ActionButton } from '../ui/workbench-controls'
import { Switch } from '../ui/switch'
import { t } from '../../../../shared/i18n'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { agentRequest, type AgentConfig } from '../../api/agent'

export function AgentAccess() {
  useTranslation()
  const [config, setConfig] = useState<AgentConfig>()
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const [installed, setInstalled] = useState(false)
  const [installFailed, setInstallFailed] = useState(false)
  useEffect(() => {
    let active = true
    agentRequest<{config: AgentConfig; skillInstalled: boolean}>('config')
      .then((result) => {
        if (!active) return
        setConfig(result.config)
        setInstalled(result.skillInstalled)
      }).catch(() => { if (active) setFailed(true) })
    return () => { active = false }
  }, [])
  async function save(next: AgentConfig) {
    setBusy(true)
    setFailed(false)
    try { await agentRequest('save', next); setConfig(next) }
    catch { setFailed(true) }
    finally { setBusy(false) }
  }
  async function installSkill() {
    setBusy(true)
    setInstallFailed(false)
    try {
      await agentRequest('installSkill')
      setInstalled(true)
    } catch {
      setInstallFailed(true)
    } finally {
      setBusy(false)
    }
  }
  const label = t('native:agent.enable')
  return (
    <>
      <SettingsGroup title={t('native:agent.access')}>
        <SettingRow label={label} error={failed ? t('settings:unableToSavePleaseTryAgain') : undefined}>
          <Switch
            aria-label={label}
            checked={config?.enabled ?? false}
            disabled={busy || !config}
            onCheckedChange={(enabled) => void save({ enabled })}
          />
        </SettingRow>
      </SettingsGroup>
      {config?.enabled && (
        <SettingsGroup title="aterm-ssh" footer={t('native:agent.skillHelp')}>
          <SettingRow label={t('native:agent.skillLabel')} error={installFailed ? t('native:agent.skillError') : undefined}>
            <ActionButton disabled={busy || installed} onClick={() => void installSkill()}>
              {t(installed ? 'native:agent.skillInstalled' : 'native:agent.skillInstall')}
            </ActionButton>
          </SettingRow>
        </SettingsGroup>
      )}
    </>
  )
}
