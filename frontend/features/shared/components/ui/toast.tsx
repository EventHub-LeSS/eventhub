"use client"

import { Toast as ToastPrimitive } from "@base-ui/react/toast"
import type { ToastObject } from "@base-ui/react/toast"
import { CheckCircle2Icon, XIcon } from "lucide-react"

import { cn } from "@/features/shared/lib/utils"

/** Module-level so any client component can enqueue a toast without going through context. */
const toastManager = ToastPrimitive.createToastManager()

function Toaster() {
  return (
    <ToastPrimitive.Provider toastManager={toastManager}>
      <ToastViewport />
    </ToastPrimitive.Provider>
  )
}

function ToastViewport() {
  const { toasts } = ToastPrimitive.useToastManager()

  return (
    <ToastPrimitive.Portal>
      <ToastPrimitive.Viewport className="fixed top-4 right-4 z-50 flex w-full max-w-sm flex-col gap-2 outline-none sm:top-6 sm:right-6">
        {toasts.map((toast) => (
          <ToastItem key={toast.id} toast={toast} />
        ))}
      </ToastPrimitive.Viewport>
    </ToastPrimitive.Portal>
  )
}

function ToastItem({ toast }: { toast: ToastObject<any> }) {
  return (
    <ToastPrimitive.Root
      toast={toast}
      data-type={toast.type}
      className={cn(
        "group/toast relative flex w-full items-start gap-3 rounded-lg border bg-popover p-4 text-popover-foreground shadow-lg ring-1 ring-foreground/10 select-none",
        "data-[type=success]:border-green-600/30 data-[type=success]:bg-green-50 data-[type=success]:text-green-900 dark:data-[type=success]:bg-green-950 dark:data-[type=success]:text-green-100",
        "data-[swipe-direction=right]:data-ending-style:translate-x-full data-[swipe-direction=down]:data-ending-style:translate-y-full",
        "data-starting-style:translate-y-1/2 data-starting-style:opacity-0 data-ending-style:opacity-0 transition-all duration-200"
      )}
    >
      {toast.type === "success" ? (
        <CheckCircle2Icon className="mt-0.5 size-5 shrink-0 text-green-600 dark:text-green-500" />
      ) : null}
      <div className="flex flex-1 flex-col gap-0.5">
        <ToastPrimitive.Title className="text-sm font-medium" />
        <ToastPrimitive.Description className="text-sm text-muted-foreground group-data-[type=success]/toast:text-green-700 dark:group-data-[type=success]/toast:text-green-300" />
      </div>
      <ToastPrimitive.Close
        className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        aria-label="Dismiss"
      >
        <XIcon className="size-4" />
      </ToastPrimitive.Close>
    </ToastPrimitive.Root>
  )
}

export { Toaster, toastManager as toast }
