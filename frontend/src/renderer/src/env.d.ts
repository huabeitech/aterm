/// <reference types="vite/client" />

import type { DesktopApi } from '../../shared/contracts/app'

declare global {
  interface Window {
    desktop: DesktopApi
  }
}
