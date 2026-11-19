import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { desktop } from '../../api/app'
import { AppProviders } from '../../app/AppProviders'
import { AppAppearance } from '../../app/AppAppearance'
import type { Bootstrap } from '../../../../shared/contracts/app'
import type { FileEditorState } from '../../../../shared/contracts/fileEditor'
import { ActionButton, Notice } from '@/components/ui/workbench-controls'
import RemoteFileEditor from './RemoteFileEditor'
import './files.css'

export default function FileEditorWindow() {
  useTranslation()
  const [bootstrap, setBootstrap] = useState<Bootstrap>()
  const [state, setState] = useState<FileEditorState>()
  const [error, setError] = useState('')
  // Serialize content updates before save/close so a final keystroke cannot be lost.
  const queue = useRef(Promise.resolve())
  const report = (error: unknown) =>
    setError(error instanceof Error ? error.message : t('files:fileEditorOperationFailed'))
  const run = (operation: () => Promise<void>) => {
    queue.current = queue.current.then(operation).catch(report)
  }
  useEffect(() => {
    const unclose = desktop.fileEditor.onCloseRequested(() => run(() => desktop.fileEditor.close()))
    const unsubscribe = desktop.fileEditor.onState((value) => {
      setState((previous) => ({ ...value, text: previous?.text ?? value.text }))
    })
    void Promise.all([desktop.bootstrap(), desktop.fileEditor.state()])
      .then(([boot, value]) => {
        setBootstrap(boot)
        setState(value)
      })
      .catch(report)
    const syncPreferences = () => {
      void desktop.bootstrap().then(setBootstrap).catch(report)
    }
    window.addEventListener('focus', syncPreferences)
    const unpackages = desktop.themes.onChanged(syncPreferences)
    const unlocale = desktop.onLocaleChanged(syncPreferences)
    const untheme = desktop.onThemeChanged((resolvedTheme) => {
      void desktop
        .bootstrap()
        .then((boot) => setBootstrap({ ...boot, resolvedTheme }))
        .catch(report)
    })
    const uncommand = desktop.onCommand((command) => {
      if (command === 'find')
        document.querySelector<HTMLButtonElement>('[data-action="find-file"]')?.click()
    })
    return () => {
      unpackages()
      unlocale()
      window.removeEventListener('focus', syncPreferences)
      unclose()
      unsubscribe()
      untheme()
      uncommand()
    }
  }, [])
  useLayoutEffect(() => {
    if (bootstrap)
      document.documentElement.style.setProperty(
        '--app-font-scale',
        String(bootstrap.preferences.appFontScale / 100)
      )
  }, [bootstrap])
  return (
    <AppProviders>
      <AppAppearance
        theme={bootstrap?.preferences.theme ?? 'system'}
        appearance={bootstrap?.resolvedTheme ?? 'light'}
      />
      <section
        className="files-workspace files-editor flex h-screen min-h-0 flex-col bg-[var(--files-bg)]"
        data-platform={bootstrap?.info.platform}
        aria-label={t('files:remoteTextEditor')}
        onKeyDownCapture={(event) => {
          if (event.nativeEvent.isComposing || !(event.metaKey || event.ctrlKey) || event.altKey)
            return
          const key = event.key.toLowerCase()
          if (key === 's' || key === 'w' || key === 'q') {
            event.preventDefault()
            event.stopPropagation()
            if (!event.repeat)
              run(() =>
                key === 's'
                  ? desktop.fileEditor.save()
                  : key === 'w'
                    ? desktop.fileEditor.close()
                    : desktop.fileEditor.quit()
              )
          }
        }}
      >
        {error && <Notice tone="error" title={t('files:fileEditorError')} description={error} />}
        {state ? (
          <>
            {!state.connected && (
              <Notice
                tone="error"
                title={t('files:sftpConnectionClosed')}
                description={t('files:yourEditsAreKeptHereCopyThemBeforeClosingThisWindow')}
              />
            )}
            <RemoteFileEditor
              path={state.document.path}
              host={state.host}
              initialValue={state.text}
              readOnly={state.saving}
              dirty={state.text !== state.document.text}
              canSave={state.connected && !state.saving && state.text !== state.document.text}
              error={state.error}
              onSave={() => run(() => desktop.fileEditor.save())}
              onClose={() => run(() => desktop.fileEditor.close())}
              onChange={(text) => {
                setState((previous) => previous && { ...previous, text })
                run(() => desktop.fileEditor.change(text))
              }}
            />
          </>
        ) : (
          <div className="p-4" role="status">
            {error ? (
              <ActionButton onClick={() => location.reload()}>{t('files:retry')}</ActionButton>
            ) : (
              t('files:loadingEditor')
            )}
          </div>
        )}
      </section>
    </AppProviders>
  )
}
