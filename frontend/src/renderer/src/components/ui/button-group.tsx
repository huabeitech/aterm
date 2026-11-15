import * as React from 'react'
import { cn } from '@/lib/utils'

function ButtonGroup({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="button-group"
      role="group"
      className={cn(
        'flex w-fit items-stretch [&>button]:-ml-px [&>button]:rounded-none [&>button:first-child]:ml-0 [&>button:first-child]:rounded-l-md [&>button:last-child]:rounded-r-md [&>button]:focus-visible:relative [&>button]:focus-visible:z-10',
        className
      )}
      {...props}
    />
  )
}

export { ButtonGroup }
