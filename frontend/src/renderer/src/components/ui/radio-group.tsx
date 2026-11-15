'use client'

import * as React from 'react'
import { Radio as RadioPrimitive, RadioGroup as RadioGroupPrimitive } from '@base-ui/react'
import { Circle } from 'lucide-react'
import { cn } from '@/lib/utils'

function RadioGroup({ className, ...props }: React.ComponentProps<typeof RadioGroupPrimitive>) {
  return (
    <RadioGroupPrimitive
      data-slot="radio-group"
      className={cn('grid gap-2', className)}
      {...props}
    />
  )
}

function RadioGroupItem({ className, ...props }: React.ComponentProps<typeof RadioPrimitive.Root>) {
  return (
    <RadioPrimitive.Root
      data-slot="radio-group-item"
      className={cn(
        'aspect-square size-4 shrink-0 rounded-full border border-input text-primary shadow-xs outline-none transition-colors',
        'focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50',
        'data-[checked]:border-primary data-[checked]:bg-primary',
        className
      )}
      {...props}
    >
      <RadioPrimitive.Indicator className="flex size-full items-center justify-center text-primary-foreground">
        <Circle className="size-2 fill-current" />
      </RadioPrimitive.Indicator>
    </RadioPrimitive.Root>
  )
}

export { RadioGroup, RadioGroupItem }
