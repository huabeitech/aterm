import { type ReactNode } from 'react'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ConfirmationProvider } from '@/components/ui/confirm-provider'
import { Toaster } from '@/components/ui/toast'
export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <TooltipProvider delayDuration={400}>
      <Toaster>
        <ConfirmationProvider>{children}</ConfirmationProvider>
      </Toaster>
    </TooltipProvider>
  )
}
