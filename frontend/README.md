# React + TypeScript + Vite + shadcn/ui

This is a template for a new Vite project with React, TypeScript, and shadcn/ui.

## Project structure

Application code is organized by feature under `src/features`:

```text
src/features/
	app/
		App.tsx
		index.ts
	shared/
		components/
		lib/
		index.ts
	src/router.tsx
```

Every feature must define an `index.ts` as its public API. Cross-feature imports
must use that barrel instead of importing internal files:

```tsx
import { App } from "@/features/app"
import { Button, ThemeProvider } from "@/features/shared"
```

Add exports deliberately when another feature needs them. Shared components and
utilities belong in `features/shared` and may be imported by every feature.

## Routing

The app uses TanStack Router. Define the route tree in `src/router.tsx` and
import route components through their feature's public `index.ts`. The root `/`
route currently renders the app feature.

## Code quality

The project uses [Oxlint](https://oxc.rs/docs/guide/usage/linter.html) and
[Oxfmt](https://oxc.rs/docs/guide/usage/formatter.html). Oxfmt also sorts imports
and Tailwind CSS classes.

```bash
bun run lint
bun run lint:fix
bun run format
bun run format:check
```

## Adding components

To add components to your app, run the following command:

```bash
npx shadcn@latest add button
```

This places UI components in `src/features/shared/components/ui`. Export any
component used outside `shared` from `src/features/shared/index.ts`.

## Using components

To use the components in your app, import them as follows:

```tsx
import { Button } from "@/features/shared"
```
