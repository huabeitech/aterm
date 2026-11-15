import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import './monacoEnvironment'
import * as monaco from 'monaco-editor'
import { useEffect, useRef, useState } from 'react'
import { ActionButton, Notice } from '@/components/ui/workbench-controls'
import { Tip } from '@/components/ui/desktop'
import { FileText, Search, WrapText, Replace, ListOrdered, X, ChevronDown } from 'lucide-react'
import { fileSize } from './fileFormat'

interface Props {
  path: string
  initialValue: string
  readOnly: boolean
  dirty: boolean
  canSave: boolean
  error: string
  onSave: () => void
  onClose: () => void
  host?: string
  onChange: (text: string) => void
}

function detectLanguage(path: string, text: string) {
  const name = path.split('/').at(-1)!.toLowerCase()
  const languages = monaco.languages.getLanguages()
  const filenameMatch = languages.find((language) =>
    language.filenames?.some((filename) => filename.toLowerCase() === name)
  )
  if (filenameMatch) return filenameMatch.id
  const extensionMatch = languages
    .flatMap((language) =>
      (language.extensions ?? []).map((extension) => ({ extension, id: language.id }))
    )
    .sort((a, b) => b.extension.length - a.extension.length)
    .find(({ extension }) => name.endsWith(extension.toLowerCase()))
  if (extensionMatch) return extensionMatch.id
  if (/^#!.*\b(bash|sh|zsh|fish)\b/.test(text)) return 'shell'
  if (/^#!.*\bpython[\d.]*\b/.test(text)) return 'python'
  if (/^(\.env(?:\..*)?|\.bashrc|\.zshrc|\.profile)$/.test(name)) return 'shell'
  return 'plaintext'
}

const languageOptions = monaco.languages
  .getLanguages()
  .map(({ id, aliases }) => ({ id, label: aliases?.[0] ?? id }))
  .sort((a, b) => a.label.localeCompare(b.label))

export default function RemoteFileEditor({
  path,
  initialValue,
  readOnly,
  dirty,
  canSave,
  error,
  onSave,
  onClose,
  onChange,
  host
}: Props) {
  const { i18n } = useTranslation()
  useEffect(() => {
    editorRef.current?.updateOptions({ ariaLabel: t('files:fileContents') })
  }, [i18n.language])
  const container = useRef<HTMLDivElement>(null)
  const editorRef = useRef<monaco.editor.IStandaloneCodeEditor>()
  const changeRef = useRef(onChange)
  changeRef.current = onChange
  const [language, setLanguage] = useState(() => detectLanguage(path, initialValue))
  const [wrap, setWrap] = useState(false)
  const [status, setStatus] = useState({
    line: 1,
    column: 1,
    lines: 1,
    bytes: 0,
    eol: 'LF',
    tabSize: 2,
    spaces: true
  })

  useEffect(() => {
    const model = monaco.editor.createModel(initialValue, detectLanguage(path, initialValue))
    const editor = monaco.editor.create(container.current!, {
      model,
      ariaLabel: t('files:fileContents'),
      automaticLayout: true,
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      tabSize: 2,
      insertSpaces: true,
      detectIndentation: true,
      fontFamily: 'Menlo, Monaco, Consolas, monospace',
      fontSize: 14,
      lineHeight: 24,
      padding: { top: 16, bottom: 16 },
      readOnly,
      contextmenu: false,
      fixedOverflowWidgets: true,
      wordWrap: 'off'
    })
    editorRef.current = editor
    model.detectIndentation(true, 2)
    let bytes = new TextEncoder().encode(initialValue).length
    const updateStatus = () => {
      const position = editor.getPosition()!
      const options = model.getOptions()
      setStatus({
        line: position.lineNumber,
        column: position.column,
        lines: model.getLineCount(),
        bytes,
        eol: model.getEOL() === '\r\n' ? 'CRLF' : 'LF',
        tabSize: options.tabSize,
        spaces: options.insertSpaces
      })
    }
    const syncAppearance = () => {
      const root = document.documentElement
      const dark = root.dataset.theme === 'dark'
      const styles = getComputedStyle(container.current!)
      monaco.editor.defineTheme('aterm-editor', {
        base: dark ? 'vs-dark' : 'vs',
        inherit: true,
        rules: [],
        colors: {
          'editor.background': styles.getPropertyValue('--files-bg').trim(),
          'editor.foreground': styles.getPropertyValue('--mac-text').trim()
        }
      })
      monaco.editor.setTheme('aterm-editor')
      const scale =
        Number.parseFloat(getComputedStyle(root).getPropertyValue('--app-font-scale')) || 1
      editor.updateOptions({ fontSize: 14 * scale, lineHeight: 24 * scale })
    }
    syncAppearance()
    const observer = new MutationObserver(syncAppearance)
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme', 'data-theme-id', 'style']
    })
    const subscriptions = [
      model.onDidChangeContent(() => {
        // preserveBOM also keeps UTF-8 BOMs intact when editing existing files.
        const text = model.getValue(undefined, true)
        bytes = new TextEncoder().encode(text).length
        changeRef.current(text)
        updateStatus()
      }),
      editor.onDidChangeCursorPosition(updateStatus),
      model.onDidChangeOptions(updateStatus)
    ]
    updateStatus()
    editor.focus()
    return () => {
      observer.disconnect()
      subscriptions.forEach((subscription) => subscription.dispose())
      editor.dispose()
      model.dispose()
      editorRef.current = undefined
    }
    // The parent keys this component by file path; saving must retain undo history.
  }, [])

  useEffect(() => {
    editorRef.current?.updateOptions({ readOnly })
  }, [readOnly])
  useEffect(() => {
    editorRef.current?.updateOptions({ wordWrap: wrap ? 'on' : 'off' })
  }, [wrap])
  useEffect(() => {
    const model = editorRef.current?.getModel()
    if (model) monaco.editor.setModelLanguage(model, language)
  }, [language])

  const action = (id: string) => {
    editorRef.current?.focus()
    void editorRef.current?.getAction(id)?.run()
  }
  return (
    <div className="remote-file-editor flex min-h-0 flex-1 flex-col">
      <header className="remote-editor-titlebar">
        <div className="remote-editor-document" title={path}>
          <FileText className="remote-editor-file-icon" size={18} aria-hidden />
          <div className="remote-editor-document-label">
            <div className="remote-editor-document-name">
              <strong>{path.split('/').at(-1)}</strong>
              <span
                className="remote-editor-save-state"
                aria-label={dirty ? t('files:unsavedChanges') : t('files:saved')}
              >
                {dirty && <span className="remote-editor-dirty-dot" aria-hidden />}
                <span className="sr-only">
                  {dirty ? t('files:unsavedChanges') : t('files:saved')}
                </span>
              </span>
            </div>
            <span className="remote-editor-directory">
              {host && `${host} · `}
              {path.slice(0, path.lastIndexOf('/')) || '/'}
            </span>
          </div>
        </div>
        <div className="remote-editor-actions" role="toolbar" aria-label={t('files:editorTools')}>
          <Tip title={t('files:findInFileCtrlF')}>
            <ActionButton
              variant="ghost"
              data-action="find-file"
              aria-label={t('files:findInFile')}
              onClick={() => action('actions.find')}
              icon={<Search />}
            />
          </Tip>
          <Tip title={t('files:findAndReplace')}>
            <ActionButton
              variant="ghost"
              aria-label={t('files:findAndReplace')}
              onClick={() => action('editor.action.startFindReplaceAction')}
              icon={<Replace />}
            />
          </Tip>
          <Tip title={t('files:goToLine')}>
            <ActionButton
              variant="ghost"
              aria-label={t('files:goToLine')}
              onClick={() => action('editor.action.gotoLine')}
              icon={<ListOrdered />}
            />
          </Tip>
          <Tip title={t('files:wordWrap')}>
            <ActionButton
              variant="ghost"
              aria-label={t('files:wordWrap')}
              aria-pressed={wrap}
              onClick={() => setWrap(!wrap)}
              icon={<WrapText />}
            />
          </Tip>
          <span className="remote-editor-action-divider" aria-hidden />
          <Tip title={t('files:saveFileCtrlS')}>
            <ActionButton
              variant="default"
              className="remote-editor-save"
              aria-label={t('files:saveFile')}
              disabled={!canSave}
              busy={readOnly}
              onClick={onSave}
            >
              {t('files:save')}
            </ActionButton>
          </Tip>
          <Tip title={t('files:closeEditor')}>
            <ActionButton
              variant="ghost"
              aria-label={t('files:closeTextEditor')}
              disabled={readOnly}
              icon={<X />}
              onClick={onClose}
            />
          </Tip>
        </div>
      </header>
      {error && (
        <Notice
          className="files-notice"
          tone="error"
          title={t('files:unableToSaveFile')}
          description={error}
        />
      )}
      <div ref={container} className="relative min-h-0 flex-1" />
      <footer className="remote-editor-statusbar" aria-label={t('files:editorStatus')}>
        <span className="remote-editor-file-stats">
          {t('files:lineCount', { count: status.lines })} <span aria-hidden>·</span>{' '}
          {fileSize(status.bytes)}
        </span>
        <div className="remote-editor-status-details">
          <button
            type="button"
            className="remote-editor-status-action"
            aria-label={t('files:goToLine')}
            title={t('files:goToLine')}
            onClick={() => action('editor.action.gotoLine')}
          >
            {t('files:cursorPosition', { line: status.line, column: status.column })}
          </button>
          <span className="remote-editor-indent">
            {status.spaces ? t('files:spaces') : t('files:tabs')}: {status.tabSize}
          </span>
          <span>
            UTF-8 <span aria-hidden>·</span> {status.eol}
          </span>
          <label className="remote-editor-language-control">
            <select
              aria-label={t('files:fileLanguage')}
              className="remote-editor-language"
              value={language}
              onChange={(event) => setLanguage(event.target.value)}
            >
              {languageOptions.map(({ id, label }) => (
                <option key={id} value={id}>
                  {label}
                </option>
              ))}
            </select>
            <ChevronDown size={12} aria-hidden />
          </label>
        </div>
      </footer>
    </div>
  )
}
