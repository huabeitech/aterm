import { isSupportedLocale } from '../../shared/contracts/locale'
import { loadMonacoLocale } from './i18n/monaco'
import { initializeLocale, i18n } from './i18n'
import { I18nextProvider } from 'react-i18next'
import { StrictMode, lazy, Suspense } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './app/App'
import './styles/ui.css'
import './styles/wails.css'
import '@xterm/xterm/css/xterm.css'

const FileEditorWindow = lazy(() => import('./features/files/FileEditorWindow'))

async function mount() {
  try {
    await initializeLocale()
  } catch (error) {
    console.error('Unable to initialize language', error)
  }
  if (isSupportedLocale(i18n.language)) await loadMonacoLocale(i18n.language)
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <I18nextProvider i18n={i18n}>
        {location.hash === '#file-editor' ? (
          <Suspense fallback={null}>
            <FileEditorWindow />
          </Suspense>
        ) : (
          <App />
        )}
      </I18nextProvider>
    </StrictMode>
  )
}
void mount()
