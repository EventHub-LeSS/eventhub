# EventHub Frontend

The web frontend for EventHub — create, manage, and share events.

## Getting started

You need [Node.js](https://nodejs.org) (v20+). Either npm (ships with Node) or [Bun](https://bun.sh) works; Bun is recommended for noticeably faster installs and dev startup.

### Install

```bash
# with bun (recommended)
bun install

# or with npm
npm install
```

### Run the dev server

```bash
bun dev      # or: npm run dev
```

The app runs at <http://localhost:5173>.

### Other scripts

```bash
bun run build      # type-check and build for production
bun run preview    # serve the production build locally
bun run lint       # run ESLint
```

(Swap `bun` for `npm run` if you're using npm.)

## Stack

React 19 with TypeScript, bundled by Vite. Routing is handled by TanStack Router using file-based routes in `src/routes`. Styling is Tailwind CSS v4, with UI components from shadcn/ui and icons from lucide-react. Server state is managed with TanStack Query.
