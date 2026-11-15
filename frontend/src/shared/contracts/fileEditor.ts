import type { RemoteText } from './files'

export interface FileEditorState {
  document: RemoteText
  text: string
  host: string
  connected: boolean
  saving: boolean
  error: string
}
export interface FileEditorApi {
  open(sessionID: string, path: string): Promise<void>
  state(): Promise<FileEditorState>
  change(text: string): Promise<void>
  save(): Promise<void>
  close(): Promise<void>
  quit(): Promise<void>
  onCloseRequested(callback: () => void): () => void
  onState(callback: (state: FileEditorState) => void): () => void
}
