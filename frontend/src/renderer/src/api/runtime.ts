import { desktop } from './app'
export const WindowToggleMaximise = () => desktop.windowAction('maximize')
export const BrowserOpenURL = (url: string) => desktop.openExternal(url)

export const ClipboardSetText = (text: string) => desktop.library.copyText(text)
