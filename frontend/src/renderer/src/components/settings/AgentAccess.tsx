import { SettingsGroup, SettingRow } from './SettingsGroup'
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
  const [command, setCommand] = useState('')
  useEffect(() => {
    let active = true
    agentRequest<{config: AgentConfig; socket: string; helper: string}>('config')
      .then((result) => {
        if (!active) return
        setConfig(result.config)
        const quote = (s: string) => "'" + s.replaceAll("'", "'\\''") + "'"
        setCommand('codex mcp add aterm -- ' + quote(result.helper) + ' --socket ' + quote(result.socket))
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
        <SettingsGroup title="Codex">
          <pre className="settings-agent-command"><code>{command}</code></pre>
        </SettingsGroup>
      )}
    </>
  )
}
