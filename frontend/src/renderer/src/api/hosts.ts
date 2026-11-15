import { desktop } from './app'
export const ListHosts = desktop.library.listHosts
export const CreateHost = desktop.library.createHost
export const UpdateHost = desktop.library.updateHost
export const DeleteHost = (id: string) => desktop.library.deleteHosts([id])
export const DeleteHosts = desktop.library.deleteHosts
export const MoveHosts = desktop.library.moveHosts
export const ReorderHosts = desktop.library.reorderHosts
export const GetHostPassword = desktop.library.getHostPassword
export const PreviewHostSnippetImport = desktop.library.previewSnippet
export const ImportHostSnippet = desktop.library.importSnippet
