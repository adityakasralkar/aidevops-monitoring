import { useParams } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { PageHeader } from '../components/layout/PageHeader'
import { ConnectionPanel } from '../features/connect/ConnectionPanel'
import type { LanguageId } from '../features/connect/languages'
import { SetupSteps } from '../features/connect/SetupSteps'
import { useProject } from '../features/projects/queries'
import { API_URL } from '../lib/api'
import styles from './ConnectPage.module.css'

/**
 * A service name that is safe inside the shown commands: letters, digits,
 * dot, dash and underscore; anything else becomes a dash.
 */
function cleanServiceName(value: string): string {
  return value.trim().replace(/[^A-Za-z0-9._-]+/g, '-').replace(/^-+|-+$/g, '') || 'my-service'
}

/** Step-by-step guide to send a program's logs and metrics to this project. */
export function ConnectPage() {
  const { projectId } = useParams({ from: '/shell/p/$projectId/connect' })
  const project = useProject(projectId)
  const [language, setLanguage] = useState<LanguageId>('node')
  const [serviceName, setServiceName] = useState('')

  useEffect(() => {
    document.title = 'Connect app · AIDevOps'
  }, [])

  const service = cleanServiceName(serviceName)
  const title = project.data ? `Connect your app to ${project.data.name}` : 'Connect your app'

  return (
    <>
      <PageHeader
        title={title}
        subtitle="Do this once for each program you want to monitor. All programs use the same API key; each one gets its own name."
      />
      <div className={styles.columns}>
        <SetupSteps
          language={language}
          onLanguage={setLanguage}
          serviceName={serviceName}
          onServiceName={setServiceName}
          service={service}
          endpoint={API_URL}
          keyPrefix={project.data?.key_prefix}
        />
        <ConnectionPanel projectId={projectId} service={service} endpoint={API_URL} />
      </div>
    </>
  )
}
