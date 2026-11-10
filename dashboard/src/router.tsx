import { createRootRoute, createRoute, createRouter, redirect } from '@tanstack/react-router'
import { AppShell } from './components/layout/AppShell'
import { RootLayout } from './components/layout/RootLayout'
import { parseLoginSearch, requireSession, skipLoginWhenSignedIn } from './features/auth/session'
import { ProjectGate } from './features/projects/ProjectGate'
import { LoginPage } from './pages/LoginPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { PlaceholderPage } from './pages/PlaceholderPage'
import { ProjectsPage } from './pages/ProjectsPage'

// Code-based route tree. URL map:
//   /login                 log in / sign up (?expired=true&redirect=/path)
//   /projects              project list                  (signed in)
//   /account               your account                  (signed in, sidebar layout)
//   /p/$projectId[/page]   one project's pages           (signed in, sidebar layout)

const rootRoute = createRootRoute({
  component: RootLayout,
  notFoundComponent: NotFoundPage,
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/projects' })
  },
})

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: 'login',
  validateSearch: parseLoginSearch,
  beforeLoad: skipLoginWhenSignedIn,
  component: LoginPage,
})

const projectsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: 'projects',
  beforeLoad: requireSession,
  component: ProjectsPage,
})

// Pathless layout route: everything below it gets the sidebar and needs a session.
const shellRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'shell',
  beforeLoad: requireSession,
  component: AppShell,
})

const accountRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: 'account',
  component: PlaceholderPage,
  staticData: { title: 'Account' },
})

const projectRoute = createRoute({ getParentRoute: () => shellRoute, path: 'p/$projectId', component: ProjectGate })

// Generic so each path keeps its exact text type ('logs', not string);
// that is what lets <Link to="/p/$projectId/logs"> be type-checked.
const projectPage = <TPath extends string>(path: TPath, title: string) =>
  createRoute({ getParentRoute: () => projectRoute, path, component: PlaceholderPage, staticData: { title } })

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  projectsRoute,
  shellRoute.addChildren([
    accountRoute,
    projectRoute.addChildren([
      projectPage('/', 'Overview'),
      projectPage('logs', 'Logs'),
      projectPage('errors', 'Errors'),
      projectPage('metrics', 'Metrics'),
      projectPage('pipelines', 'Pipelines'),
      projectPage('connect', 'Connect app'),
      projectPage('settings', 'Settings'),
    ]),
  ]),
])

export const router = createRouter({ routeTree, defaultPreload: 'intent' })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
  // Extra info a route can carry; PlaceholderPage reads it.
  interface StaticDataRouteOption {
    title?: string
  }
}
