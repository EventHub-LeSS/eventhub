import path from "node:path"

import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { keycloakify } from "keycloakify/vite-plugin"
import { defineConfig } from "vite"

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    keycloakify({
      themeName: "eventhub",
      accountThemeImplementation: "none",
      // Read by @oussemasahbeni/keycloakify-login-shadcn at runtime, overridable per container.
      environmentVariables: [
        { name: "SHADCN_THEME_LOGO_WHITE_URL", default: "" },
        { name: "SHADCN_THEME_LOGO_DARK_URL", default: "" },
        { name: "SHADCN_THEME_APP_NAME", default: "EventHub" },
        { name: "SHADCN_THEME_LAYOUT", default: "two-column" },
        { name: "SHADCN_THEME_SIDE_IMAGE_URL", default: "" },
        { name: "SHADCN_THEME_PRESET", default: "neutral" },
        { name: "SHADCN_THEME_BASE", default: "neutral" },
        { name: "SHADCN_THEME_RADIUS", default: "default" },
        { name: "SHADCN_THEME_FONT", default: "geist" },
        { name: "SHADCN_THEME_PLACEHOLDER", default: "true" },
      ],
    }),
  ],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "src"),
    },
  },
})
