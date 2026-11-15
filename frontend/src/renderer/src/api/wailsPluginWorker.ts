// Sandbox SDK is shared behavior from the Electron worker host. Untrusted code
// runs only inside a dedicated Worker in an opaque-origin, CSP-restricted frame.
import {createPluginTranslator} from '../../../shared/plugins/localization'
import type {PluginLocalizations} from '../../../shared/contracts/plugins'
export function workerSource(source: string, localizations: PluginLocalizations | undefined, locale: string): string {
 const bootstrap = `
      const pending = new Map(); let nextId = 0; let refresh; const commands = new Map();
      const operations = new Map(); const streams = new Map();
      function enqueue(token, work) {
        const result = (operations.get(token) || Promise.resolve()).then(work);
        const tail = result.catch(() => {}); operations.set(token, tail);
        tail.then(() => { if (operations.get(token) === tail) operations.delete(token) });
        return result;
      }
      let language = ${JSON.stringify(locale)};
      const bundle = ${JSON.stringify(localizations ?? null)};
      const translatorFactory = ${createPluginTranslator.toString()};
      let translate = translatorFactory(bundle, language);
      const localeListeners = new Set();
      const api = Object.freeze({
        capabilities: Object.freeze({ hostApi: 1, statusBar: true, views: true, workspaceViews: true, processExecution: true, processOutputTail: true, processStreaming: true, interactiveTerminals: true, commands: true, storage: true }),
        i18n: Object.freeze({
          get locale() { return language },
          t: (key, options) => translate(key, options),
          onDidChangeLocale(callback) { localeListeners.add(callback); return () => localeListeners.delete(callback) }
        }),
        commands: Object.freeze({ registerCommand(id, callback) { commands.set(id, callback); return () => commands.delete(id) } }),
        storage: Object.freeze({
          async get(key) { const r = await api.request('storage.get', { key }); if (!r.ok) throw Error('Storage read failed'); return r.value },
          async set(key, value) { const r = await api.request('storage.set', { key, value }); if (!r.ok) throw Error('Storage write failed') }
        }),
        process: Object.freeze({
        async subscribe({ token, ...request }, callback) {
          const id = 'stream-' + Date.now() + '-' + (++nextId);
          streams.set(id, { token, callback });
          const r = await api.request('process.stream.start', { token, id, request });
          if (!r.ok || r.value?.error) { streams.delete(id); throw Error(r.value?.error || 'Stream unavailable'); }
          return { async dispose() { const s = streams.get(id); if (s) s.callback = undefined; await api.request('process.stream.stop', { token, id }); } };
        },
        async run({ token, ...request }) {
          const r = await api.request('process.execute', { token, request });
          if (!r.ok) throw Error('Execution permission or session unavailable');
          if (r.value?.error) throw Error(r.value.error);
          return r.value;
        } }),
        terminals: Object.freeze({ async open({ token, ...request }) {
          const r = await api.request('terminal.create', { token, request });
          if (!r.ok) throw Error('Terminal permission or session unavailable');
          if (r.value?.error) throw Error(r.value.error);
          return r.value;
        } }),
        onRefresh(callback) { refresh = callback },
        request(method, params) {
          const requestId = ++nextId;
          return new Promise(resolve => { pending.set(requestId, resolve); postMessage({ type: 'request', requestId, method, params }) });
        }
      });
      Object.defineProperty(globalThis, 'atermPlugin', { value: api });
      Object.defineProperty(globalThis, 'window', { value: Object.freeze({ atermPlugin: api }) });
      Object.defineProperty(globalThis, 'Worker', { value: undefined });
      Object.defineProperty(globalThis, 'SharedWorker', { value: undefined });
      onmessage = ({ data }) => {
        if (data.type === 'process-stream') {
          const event = data.event, subscription = streams.get(event.id);
          if (subscription) Promise.resolve().then(() => subscription.callback?.(event)).catch(() => {
            subscription.callback = undefined; void api.request('process.stream.stop', { token: subscription.token, id: event.id });
          }).finally(() => {
            if (event.done) streams.delete(event.id);
            void api.request('process.stream.ack', { token: subscription.token, id: event.id });
          });
        }
        if (data.type === 'locale') { language = data.locale; translate = translatorFactory(bundle, language); for (const listener of localeListeners) { try { listener(language) } catch {} } }
        if (data.type === 'ping') postMessage({ type: 'request', requestId: 0, method: 'runtime.heartbeat', params: {} });
        if (data.type === 'response') { const resolve = pending.get(data.requestId); pending.delete(data.requestId); resolve?.(data.result) }
        if (data.type === 'command') enqueue(data.context.token, () => {
          const handler = commands.get(data.context.command);
          if (!handler) throw Error('Command handler not registered');
          return handler(data.context);
        }).then(value => api.request('runtime.commandResult', { callId: data.context.callId, value: value ?? null }),
          error => api.request('runtime.commandResult', { callId: data.context.callId, error: String(error?.message || 'Command failed') }));
        if (data.type === 'refresh') enqueue(data.context.token, () => refresh?.(data.context)).catch(() => {});
      };
    `
 return bootstrap + '\n;(function () {\n' + source + '\n})();'
}
