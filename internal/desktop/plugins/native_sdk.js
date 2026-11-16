(() => {
  const requestHost = globalThis.__request
  const plural = globalThis.__plural
  const {localizations, locale: initialLocale} = JSON.parse(globalThis.__config)
  let locale = initialLocale
  const localeListeners = new Set()
  delete globalThis.__request
  delete globalThis.__config
  delete globalThis.__plural

  const commands = new Map()
  const streams = new Map()
  let refresh
  let sequence = 0

  function request(method, params) {
    return JSON.parse(requestHost(method, JSON.stringify(params)))
  }

  async function processRequest(method, params) {
    const result = request(method, params)
    if (!result.ok) throw Error('Execution permission or session unavailable')
    if (result.value?.error) throw Error(result.value.error)
    return result.value
  }

  function translate(key, options = {}) {
    const languages = new Set([locale, locale.split('-')[0], localizations?.defaultLocale])
    for (const language of languages) {
      const messages = localizations?.messages[language]
      if (!messages) continue
      const count = options.count
      const message = (typeof count === 'number' && Number.isFinite(count)
        ? messages[key + '_' + plural(language, count)] ?? messages[key + '_other']
        : undefined) ?? messages[key]
      if (typeof message === 'string') {
        return message.replace(/{{\s*([\w.-]+)\s*}}/g, (match, name) =>
          Object.hasOwn(options, name) ? String(options[name]) : match)
      }
    }
    return key
  }

  const api = Object.freeze({
    capabilities: Object.freeze({hostApi: 1, statusBar: true, views: true, workspaceViews: true, processExecution: true, processOutputTail: true, processStreaming: true, interactiveTerminals: true, commands: true, storage: true}),
    i18n: Object.freeze({
      get locale() { return locale },
      t: translate,
      onDidChangeLocale(callback) {
        localeListeners.add(callback)
        return () => localeListeners.delete(callback)
      },
    }),
    commands: Object.freeze({
      registerCommand(id, callback) {
        commands.set(id, callback)
        return () => commands.delete(id)
      },
    }),
    storage: Object.freeze({
      async get(key) {
        const result = request('storage.get', {key})
        if (!result.ok) throw Error('Storage read failed')
        return result.value
      },
      async set(key, value) {
        if (!request('storage.set', {key, value}).ok) throw Error('Storage write failed')
      },
    }),
    process: Object.freeze({
      run({token, ...params}) { return processRequest('process.execute', {token, request: params}) },
      async subscribe({token, ...params}, callback) {
        const id = 'stream-' + ++sequence
        streams.set(id, {token, callback})
        try {
          await processRequest('process.stream.start', {token, id, request: params})
        } catch (error) {
          streams.delete(id)
          throw error
        }
        return {async dispose() {
          const stream = streams.get(id)
          if (stream) stream.callback = undefined
          await processRequest('process.stream.stop', {token, id})
        }}
      },
    }),
    terminals: Object.freeze({
      open({token, ...params}) { return processRequest('terminal.create', {token, request: params}) },
    }),
    async request(method, params) {
      if (method === 'runtime.ready') return {ok: true}
      if (method.startsWith('runtime.')) return {ok: false}
      return request(method, params)
    },
    onRefresh(callback) { refresh = callback },
  })
  Object.defineProperty(globalThis, 'atermPlugin', {value: api})
  Object.defineProperty(globalThis, 'window', {value: Object.freeze({atermPlugin: api})})

  return async message => {
    const {kind, context, event, locale: nextLocale} = JSON.parse(message)
    if (kind === 'locale') {
      locale = nextLocale
      for (const listener of localeListeners) {
        try { await listener(locale) } catch {}
      }
    }
    if (kind === 'refresh') {
      try { await refresh?.(context) } catch {}
    }
    if (kind === 'command') {
      const {callId, command} = context
      try {
        const handler = commands.get(command)
        if (!handler) throw Error('Command handler not registered')
        const value = await handler(context)
        request('runtime.commandResult', {callId, value: value ?? null})
      } catch (error) {
        request('runtime.commandResult', {callId, error: String(error?.message || 'Command failed')})
      }
    }
    if (kind === 'process-stream') {
      const stream = streams.get(event.id)
      if (!stream) return
      try {
        await stream.callback?.(event)
      } catch {
        stream.callback = undefined
        request('process.stream.stop', {token: stream.token, id: event.id})
      } finally {
        if (event.done) streams.delete(event.id)
        request('process.stream.ack', {token: stream.token, id: event.id})
      }
    }
  }
})()
