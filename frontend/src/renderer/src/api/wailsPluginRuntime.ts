import {workerSource} from './wailsPluginWorker'
import type {PluginLocalizations} from '../../../shared/contracts/plugins'
interface Control {kind: string; instanceId: string; source?: string; localizations?: PluginLocalizations; locale?: string; context?: unknown; event?: unknown}
interface Sandbox {frame: HTMLIFrameElement; port: MessagePort; heartbeat: ReturnType<typeof setInterval>; lastBeat: number; lastCheck: number; active: number; calls: number; second: number; disposed: boolean}
type Invoke = (channel: string, ...args: unknown[]) => Promise<any>
function dataURL(source: string, type = 'text/javascript'): string {
 const bytes = new TextEncoder().encode(source); let binary = ''
 for (let offset = 0; offset < bytes.length; offset += 0x8000) binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000))
 return `data:${type};base64,${btoa(binary)}`
}
// Only this trusted transport knows a sandbox's instance identity and binding.
// The worker receives neither Wails globals nor a reference to the app window.
export function createPluginRuntime(invoke: Invoke) {
 const sandboxes = new Map<string, Sandbox>()
 function stop(id: string) {
  const sandbox = sandboxes.get(id); if (!sandbox) return
  sandboxes.delete(id); sandbox.disposed = true; clearInterval(sandbox.heartbeat)
  sandbox.port.postMessage({type: 'stop'}); sandbox.port.close(); sandbox.frame.remove()
 }
 function fault(id: string) { stop(id); void invoke('plugins:runtime:rpc', id, 'runtime.fault', {}).catch(() => {}) }
 async function start(control: Control) {
  stop(control.instanceId)
  const id = control.instanceId, frame = document.createElement('iframe'), channel = new MessageChannel()
  frame.hidden = true; frame.setAttribute('aria-hidden', 'true'); frame.sandbox.add('allow-scripts')
  const workerURL = dataURL(workerSource(control.source ?? '', control.localizations, control.locale ?? 'en'))
  // The frame's only role is a trusted relay. Plugin source never enters its DOM.
  const hostSource = `
   let initialized = false;
   addEventListener('message', ({source, data, ports}) => {
    if (initialized || source !== parent || data?.type !== 'initialize' || !ports[0]) return;
    initialized = true; const port = ports[0]; let worker;
    try {
     worker = new Worker(${JSON.stringify(workerURL)});
     worker.onmessage = ({data}) => port.postMessage(data);
     worker.onerror = () => port.postMessage({type:'fault'});
     port.onmessage = ({data}) => { if (data?.type === 'stop') { worker.terminate(); port.close(); } else worker.postMessage(data); };
     port.start();
    } catch { port.postMessage({type:'fault'}); }
   });`
  const policy = "default-src 'none'; script-src data:; connect-src 'none'; frame-src 'none'; worker-src data:; form-action 'none'; base-uri 'none'; object-src 'none'"
  const html = `<meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${policy}"><script src="${dataURL(hostSource)}"></script>`
  const sandbox: Sandbox = {frame,port:channel.port1,heartbeat:0 as unknown as ReturnType<typeof setInterval>,lastBeat:Date.now(),lastCheck:Date.now(),active:0,calls:0,second:0,disposed:false}
  sandboxes.set(id,sandbox)
  channel.port1.onmessage = ({data}) => {
   if (sandbox.disposed) return
   if (data?.type === 'fault') { fault(id); return }
   if (data?.type !== 'request' || !Number.isSafeInteger(data.requestId) || typeof data.method !== 'string') return
   let size: number; try { size = JSON.stringify(data.params).length } catch { return }
   const second = Math.floor(Date.now()/1000); if (second !== sandbox.second) { sandbox.second=second; sandbox.calls=0 }
   if (++sandbox.calls>50 || sandbox.active>=16 || size>262144) { channel.port1.postMessage({type:'response',requestId:data.requestId,result:{ok:false}}); return }
   if (data.method === 'runtime.heartbeat') sandbox.lastBeat = Date.now()
   sandbox.active++
   void invoke('plugins:runtime:rpc', id, data.method, data.params).then(value => {
    if (!sandbox.disposed) channel.port1.postMessage({type:'response',requestId:data.requestId,result:{ok:true,value}})
   }, () => {
    if (!sandbox.disposed) channel.port1.postMessage({type:'response',requestId:data.requestId,result:{ok:false}})
   }).finally(() => {sandbox.active--})
  }
  channel.port1.start()
  frame.onload = () => { if (!sandbox.disposed) frame.contentWindow?.postMessage({type:'initialize'},'*',[channel.port2]) }
  sandbox.heartbeat = setInterval(() => {
   const now=Date.now(); if (now-sandbox.lastCheck>10000) sandbox.lastBeat=now; sandbox.lastCheck=now
   if (now-sandbox.lastBeat>10000) { fault(id); return }
   channel.port1.postMessage({type:'ping'})
  },3000)
  const url = await invoke('plugins:runtime:document',id,html) as string
  if (!sandbox.disposed) {frame.src=url;document.body.append(frame)}
 }
 function control(event: Control) {
  if (event.kind==='start') { void start(event).catch(() => fault(event.instanceId)); return }
  if (event.kind==='stop') { stop(event.instanceId); return }
  const sandbox=sandboxes.get(event.instanceId); if (!sandbox) return
  if (event.kind==='command'||event.kind==='refresh') sandbox.port.postMessage({type:event.kind,context:event.context})
  else if (event.kind==='process-stream') sandbox.port.postMessage({type:'process-stream',event:event.event})
 }
 return {control,dispose(){for (const id of sandboxes.keys()) stop(id)}}
}
