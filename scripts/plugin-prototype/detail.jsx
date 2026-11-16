import React, {useEffect, useRef, useState} from 'react'
import {createRoot} from 'react-dom/client'

function Details() {
  const [snapshot, setSnapshot] = useState(null)
  const channel = useRef(null)
  useEffect(() => {
    if (LIVE_SSH) {
      const update = state => setSnapshot(state?.status?.data ?? null)
      update(window.atermView.state)
      return window.atermView.onDidChange(update)
    }
    let port
    function connect(event) {
      if (event.source !== parent || event.data !== 'initialize' || event.ports.length !== 1) return
      port = event.ports[0]
      channel.current = port
      port.onmessage = ({data}) => {
        if (data.type === 'snapshot') setSnapshot(data.snapshot)
      }
      port.postMessage({type: 'command', id: 'monitor.snapshot'})
      window.removeEventListener('message', connect)
    }
    window.addEventListener('message', connect)
    parent.postMessage('detail-ready', '*')
    return () => {
      window.removeEventListener('message', connect)
      port?.close()
    }
  }, [])

  useEffect(() => {
    if (!snapshot) return
    // This concerns browser isolation. The harness exposes no Go services.
    let parentBlocked = false
    try { void parent.document } catch { parentBlocked = true }
    const report = {
      type: 'rendered',
      title: document.querySelector('h1').textContent,
      metric: document.querySelector('output').textContent,
      parentBlocked,
      opaqueOrigin: self.origin === 'null',
      runtimeAbsent: typeof window._wails === 'undefined',
    }
    if (LIVE_SSH) parent.postMessage({channel: 'resource-monitor-acceptance', ...report}, '*')
    else channel.current.postMessage(report)
  }, [snapshot])

  return <main>
    <h1>{DETAIL_TITLE}</h1>
    {snapshot && <><label htmlFor="metric">{METRIC_LABEL}</label><output id="metric">{snapshot.value} MiB</output></>}
  </main>
}

createRoot(document.getElementById('root')).render(<Details />)
