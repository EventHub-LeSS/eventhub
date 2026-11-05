# Next.js template

This is a Next.js template with shadcn/ui.

## Adding components

To add components to your app, run the following command:

```bash
bunx --bun shadcn@latest add button
```

This will place the ui components in the `components` directory.

## Using components

To use the components in your app, import them as follows:

```tsx
import { Button } from "@/components/ui/button"
```

## Checking the 404 page

The root `app/not-found.tsx` uses the existing layout, navigation, and theme
provider. Keep it free of document-level colour overrides.

After `bun run build` and `bun run start`, open an unknown URL with the system
appearance set to dark and the app theme set to light. The page and header must
remain light and readable. Also check dark and system themes, navigation and
reload, mobile layout, and the "Zur Startseite" link. A fresh request to an
unknown route must return HTTP 404.
