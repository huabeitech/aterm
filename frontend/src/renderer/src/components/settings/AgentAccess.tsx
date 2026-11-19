import { t } from '../../../../shared/i18n'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { agentRequest, type AgentConfig } from '../../api/agent'
import { ListHosts } from '../../api/hosts'
import type { domain } from '../../api/types'

export function AgentAccess() {
  useTranslation()
  const [config, setConfig] = useState<AgentConfig>()
  const [hosts, setHosts] = useState<domain.Host[]>([])
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const [command, setCommand] = useState('')
  useEffect(() => {
    let active = true
    Promise.all([agentRequest<{config: AgentConfig; socket: string; helper: string}>('config'), ListHosts()])
      .then(([result, hosts]) => {
        if (!active) return
        setConfig(result.config)
        setHosts(hosts)
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
  return <fieldset className="settings-group" disabled={busy || !config}>
    <legend>{t('native:agent.access')}</legend>
    <label><input type="checkbox" checked={!!config?.enabled}
      onChange={e => config && void save({...config, enabled: e.target.checked})} />
      {t('native:agent.enable')}</label>
    {config?.enabled && <>
      {hosts.map(h => <div key={h.id}><label><input type="checkbox" checked={!!config.hosts?.[h.id]}
        onChange={e => void save({...config, hosts: {...config.hosts, [h.id]:e.target.checked}})} />
        {h.name || h.alias || h.hostName}</label></div>)}
      <pre style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere',userSelect:'all'}}>{command}</pre>
    </>}
    {failed && <p role="alert">{t('settings:unableToSavePleaseTryAgain')}</p>}
  </fieldset>
}
