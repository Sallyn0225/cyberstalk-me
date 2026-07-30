import * as React from 'react'
import { Label as LabelPrimitive } from 'radix-ui'

import { cn } from '@/lib/utils'

/**
 * The form primitives.
 *
 * The layout is fixed on purpose: label above the control, helper text under
 * the label, error text below the control. A placeholder is a hint, never a
 * label, so every control here has a real <label> tied to it.
 */

function Label({
  className,
  ...props
}: React.ComponentProps<typeof LabelPrimitive.Root>) {
  return (
    <LabelPrimitive.Root
      data-slot="label"
      className={cn(
        'text-sm font-medium text-foreground select-none peer-disabled:opacity-60',
        className,
      )}
      {...props}
    />
  )
}

function Input({
  className,
  ...props
}: React.ComponentProps<'input'>) {
  return (
    <input
      data-slot="input"
      // None of these fields are credentials a password manager should offer
      // to fill; the device token least of all.
      autoComplete="off"
      className={cn(
        'h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-xs transition-colors',
        'placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 outline-none',
        'disabled:cursor-not-allowed disabled:opacity-50',
        'aria-invalid:border-destructive aria-invalid:ring-destructive/30',
        className,
      )}
      {...props}
    />
  )
}

/** Field stacks a label, its control, and its messages in the fixed order. */
function Field({
  label,
  htmlFor,
  hint,
  error,
  className,
  children,
}: {
  label: string
  htmlFor: string
  hint?: React.ReactNode
  error?: string
  className?: string
  children: React.ReactNode
}) {
  return (
    <div className={cn('grid gap-2', className)}>
      <div className="grid gap-1">
        <Label htmlFor={htmlFor}>{label}</Label>
        {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      </div>
      {children}
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : null}
    </div>
  )
}

export { Field, Input, Label }
