"use client"

import { CheckIcon, CopyIcon, MailIcon } from "lucide-react"
import { useState } from "react"

import { Button, buttonVariants } from "@/features/shared/components/ui/button"
import { Input } from "@/features/shared/components/ui/input"
import { toast } from "@/features/shared/components/ui/toast"

export function CopyInviteEmail({ email }: { email: string }) {
  const [copied, setCopied] = useState(false)

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(email)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
      toast.add({
        type: "success",
        title: "E-Mail-Adresse kopiert",
        description: "Schicke sie jetzt an deinen Organisations-Admin.",
      })
    } catch {
      // The clipboard API is unavailable outside secure contexts; let the user copy by hand.
      const input = document.getElementById("join-invite-email")
      if (input instanceof HTMLInputElement) {
        input.select()
      }
      toast.add({
        type: "error",
        title: "Kopieren nicht möglich",
        description: "Die Adresse ist markiert – kopiere sie mit Strg/⌘ + C.",
      })
    }
  }

  return (
    <div className="flex gap-2">
      <Input
        id="join-invite-email"
        readOnly
        value={email}
        aria-label="Deine E-Mail-Adresse"
        className="font-mono"
        onFocus={(event) => event.currentTarget.select()}
      />
      <Button type="button" variant="outline" onClick={handleCopy}>
        {copied ? <CheckIcon /> : <CopyIcon />}
        {copied ? "Kopiert" : "Kopieren"}
      </Button>
    </div>
  )
}

export function OpenMailButton() {
  return (
    <a href="mailto:" className={buttonVariants()}>
      <MailIcon />
      E-Mail-Programm öffnen
    </a>
  )
}
