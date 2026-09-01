import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import { KcPage } from "./kc.gen"

// Only reachable through `vite dev`; in Keycloak the page is server-rendered with a kcContext.
if (window.kcContext === undefined) {
  throw new Error(
    "No kcContext. Use `bun run storybook` to preview the pages outside of Keycloak."
  )
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <KcPage kcContext={window.kcContext} />
  </StrictMode>
)
