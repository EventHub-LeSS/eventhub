import { Geist, Geist_Mono } from "next/font/google"

import "@/app/globals.css"
import { Navbar } from "@/features/app"
import { getCurrentUser } from "@/features/auth"
import { ThemeProvider } from "@/features/shared/components/theme-provider"
import { cn } from "@/features/shared/lib/utils"

const geist = Geist({ subsets: ["latin"], variable: "--font-sans" })

const fontMono = Geist_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
})

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  const user = await getCurrentUser()
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={cn(
        "antialiased",
        fontMono.variable,
        "font-sans",
        geist.variable
      )}
    >
      <body>
        <ThemeProvider>
          <div className="flex min-h-svh flex-col">
            <Navbar user={user} />
            {children}
          </div>
        </ThemeProvider>
      </body>
    </html>
  )
}
