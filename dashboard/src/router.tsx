import { createRootRoute, createRoute, createRouter, redirect } from '@tanstack/react-router'
import { AppShell } from './components/layout/AppShell'
import { AccountPage } from './pages/AccountPage'
import { RootLayout } from './components/layout/RootLayout'
import { parseLoginSearch, requireSession, skipLoginWhenSignedIn } from './features/auth/session'
import { parseErrorSearch } from './features/errors/filters'
import { parseLogSearch } from './features/logs/filters'
import { parseMetricSearch } from './features/metrics/search'
import { parseRangeSearch } from './features/overview/range'
import { parsePipelineSearch } from './features/pipelines/queries'
import { ProjectGate } from './features/projects/ProjectGate'
import { ConnectPage } from './pages/ConnectPage'
import { ErrorsPage } from './pages/ErrorsPage'
import { LoginPage } from './pages/LoginPage'
import { LogsPage } from './pages/LogsPage'
import { MetricsPage } from './pages/MetricsPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { OverviewPage } from './pages/OverviewPage'
import { PipelinesPage } from './pages/PipelinesPage'
import { ProjectsPage } from './pages/ProjectsPage'
import { SettingsPage } from './pages/SettingsPage'

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

const accountRoute = createRoute({ getParentRoute: () => shellRoute, path: 'account', component: AccountPage })

const projectRoute = createRoute({ getParentRoute: () => shellRoute, path: 'p/$projectId', component: ProjectGate })

const overviewRoute = createRoute({
  getParentRoute: () => projectRoute,
  path: '/',
  validateSearch: parseRangeSearch,
  component: OverviewPage,
})

const logsRoute = createRoute({
  getParentRoute: () => projectRoute,
  path: 'logs',
  validateSearch: parseLogSearch,
  component: LogsPage,
})

const errorsRoute = createRoute({
  getParentRoute: () => projectRoute,
  path: 'errors',
  validateSearch: parseErrorSearch,
  component: ErrorsPage,
})

const metricsRoute = createRoute({
  getParentRoute: () => projectRoute,
  path: 'metrics',
  validateSearch: parseMetricSearch,
  component: MetricsPage,
})

const settingsRoute = createRoute({ getParentRoute: () => projectRoute, path: 'settings', component: SettingsPage })

const pipelinesRoute = createRoute({
  getParentRoute: () => projectRoute,
  path: 'pipelines',
  validateSearch: parsePipelineSearch,
  component: PipelinesPage,
})

const connectRoute = createRoute({ getParentRoute: () => projectRoute, path: 'connect', component: ConnectPage })

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  projectsRoute,
  shellRoute.addChildren([
    accountRoute,
    projectRoute.addChildren([
      overviewRoute,
      logsRoute,
      errorsRoute,
      metricsRoute,
      pipelinesRoute,
      connectRoute,
      settingsRoute,
    ]),
  ]),
])

export const router = createRouter({ routeTree, defaultPreload: 'intent' })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
