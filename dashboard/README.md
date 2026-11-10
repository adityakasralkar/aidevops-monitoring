# AIDevOps dashboard

Web dashboard for the AIDevOps monitoring server.

- **Stack:** React + Vite + TypeScript, pure CSS (CSS variables + CSS Modules)
- **Libraries:** TanStack Router (pages), TanStack Query (server data), Redux Toolkit (app state)

## Run

From the repository root:

```bash
make dashboard
```

Opens on http://localhost:5173, the origin the Go server allows by default (`DASHBOARD_ORIGINS`).

Inside `dashboard/`:

| Command | Does |
|---|---|
| `npm run dev` | Start the dev server |
| `npm run build` | Type-check and build to `dist/` |
| `npm run lint` | Lint with oxlint |
| `npm run preview` | Serve the built `dist/` |

## Layout

| Folder | Holds |
|---|---|
| `src/styles` | Design tokens (`tokens.css`) and page-wide styles |
| `src/components/ui` | Generic building blocks; no API calls |
| `src/components/layout` | App shell, sidebar, user card, page header |
| `src/features` | Code for one area of the app (auth, projects, …) |
| `src/pages` | One component per route |
| `src/store` | Redux store: session and other app state |
| `src/lib` | Shared logic without JSX (query client, types) |
| `src/router.tsx` | All routes |
