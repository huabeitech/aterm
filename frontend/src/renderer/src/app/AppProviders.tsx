import { type ReactNode } from 'react'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ConfirmationProvider } from '@/components/ui/confirm-provider'
import { Toaster } from '@/components/ui/toast'
import { DesktopConfirmations } from './DesktopConfirmations'
export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <TooltipProvider delayDuration={400}>
      <Toaster>
        <ConfirmationProvider>
          <DesktopConfirmations />
          {children}
        </ConfirmationProvider>
      </Toaster>
    </TooltipProvider>
  )
}
