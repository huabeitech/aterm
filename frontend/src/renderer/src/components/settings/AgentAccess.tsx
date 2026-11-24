import { SettingsGroup, SettingRow } from './SettingsGroup'
import { ActionButton } from '../ui/workbench-controls'
import { Switch } from '../ui/switch'
import { t } from '../../../../shared/i18n'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { agentRequest, onAgentGrantsChanged, type AgentConfig } from '../../api/agent'

type AgentGrant = { name: string; endpoint: string }
type AgentGrants = Record<string, AgentGrant> | null

export function AgentAccess() {
  useTranslation()
  const [config, setConfig] = useState<AgentConfig>()
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const [installed, setInstalled] = useState(false)
  const [installFailed, setInstallFailed] = useState(false)
  const [grants, setGrants] = useState<AgentGrants>(null)
  const [grantFailed, setGrantFailed] = useState(false)
  useEffect(() => {
    let active = true
    let revision = 0
    const reloadGrants = () => {
      const request = ++revision
      void agentRequest<AgentGrants>('grants')
        .then((result) => {
          if (active && request === revision) {
            setGrants(result)
            setGrantFailed(false)
          }
        }).catch(() => { if (active && request === revision) setGrantFailed(true) })
    }
    const unsubscribe = onAgentGrantsChanged(reloadGrants)
    reloadGrants()
    agentRequest<{config: AgentConfig; skillInstalled: boolean}>('config')
      .then((result) => {
        if (!active) return
        setConfig(result.config)
        setInstalled(result.skillInstalled)
      }).catch(() => { if (active) setFailed(true) })
    return () => { active = false; unsubscribe() }
  }, [])
  async function revokeGrant(hostID: string) {
    setBusy(true)
    setGrantFailed(false)
    try { setGrants(await agentRequest<AgentGrants>('revokeGrant', hostID)) }
    catch { setGrantFailed(true) }
    finally { setBusy(false) }
  }
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
  const entries = Object.entries(grants ?? {}).sort(([, first], [, second]) => first.name.localeCompare(second.name))
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
      {(entries.length > 0 || grantFailed) && (
        <SettingsGroup title={t('native:agent.alwaysAllow')} footer={t('native:agent.grantHelp')}>
          {entries.map(([hostID, grant]) => (
            <SettingRow key={hostID} label={<>{grant.name} <span dir="ltr">({grant.endpoint})</span></>}>
              <ActionButton disabled={busy} onClick={() => void revokeGrant(hostID)}>
                {t('native:agent.revokeGrant')}
              </ActionButton>
            </SettingRow>
          ))}
          <SettingRow label={t('native:agent.savedGrants')} error={grantFailed ? t('settings:unableToSavePleaseTryAgain') : undefined}>
            <ActionButton disabled={busy || entries.length === 0} onClick={() => void revokeGrant('')}>
              {t('native:agent.revokeAllGrants')}
            </ActionButton>
          </SettingRow>
        </SettingsGroup>
      )}
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
