// Install and run instructions per language. They all read the same
// OTEL_* settings from step 3, so only these two parts differ.

export type LanguageId = 'node' | 'python' | 'java' | 'go' | 'dotnet' | 'php' | 'other'

export interface Language {
  id: LanguageId
  label: string
  installTitle: string
  install: string
  run: string
}

export const languages: Language[] = [
  {
    id: 'node',
    label: 'Node.js',
    installTitle: 'Install the OpenTelemetry packages',
    install: 'npm install @opentelemetry/api @opentelemetry/auto-instrumentations-node',
    run: '# Starts OpenTelemetry before your app; no code changes\nnode --require @opentelemetry/auto-instrumentations-node/register app.js',
  },
  {
    id: 'python',
    label: 'Python',
    installTitle: 'Install the OpenTelemetry distro',
    install: 'pip install opentelemetry-distro opentelemetry-exporter-otlp\nopentelemetry-bootstrap -a install',
    run: 'OTEL_PYTHON_LOGGING_AUTO_INSTRUMENTATION_ENABLED=true \\\nopentelemetry-instrument python app.py',
  },
  {
    id: 'java',
    label: 'Java',
    installTitle: 'Download the OpenTelemetry Java agent',
    install:
      'curl -L -O https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/latest/download/opentelemetry-javaagent.jar',
    run: 'java -javaagent:opentelemetry-javaagent.jar -jar app.jar',
  },
  {
    id: 'go',
    label: 'Go',
    installTitle: 'Add the OpenTelemetry packages',
    install:
      'go get go.opentelemetry.io/otel \\\n  go.opentelemetry.io/otel/sdk \\\n  go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp \\\n  go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp',
    run: '# Go has no automatic agent: start the SDK in main.go, then\ngo run .',
  },
  {
    id: 'dotnet',
    label: '.NET',
    installTitle: 'Add the OpenTelemetry packages',
    install:
      'dotnet add package OpenTelemetry.Extensions.Hosting\ndotnet add package OpenTelemetry.Exporter.OpenTelemetryProtocol\ndotnet add package OpenTelemetry.Instrumentation.AspNetCore',
    run: 'dotnet run',
  },
  {
    id: 'php',
    label: 'PHP',
    installTitle: 'Install the OpenTelemetry extension and SDK',
    install: 'pecl install opentelemetry\ncomposer require open-telemetry/sdk open-telemetry/exporter-otlp',
    run: 'OTEL_PHP_AUTOLOAD_ENABLED=true php -S localhost:8000',
  },
  {
    id: 'other',
    label: 'Other',
    installTitle: 'Install the SDK for your language',
    install: '',
    run: '# Run your app as usual.\n# The SDK reads the settings from step 3.',
  },
]

/** The settings every SDK reads from the environment. */
export function otelSettings({ service, endpoint }: { service: string; endpoint: string }): string {
  return [
    `OTEL_SERVICE_NAME=${service}`,
    `OTEL_EXPORTER_OTLP_ENDPOINT=${endpoint}`,
    'OTEL_EXPORTER_OTLP_HEADERS=x-api-key=PASTE_YOUR_API_KEY',
    'OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf',
    'OTEL_TRACES_EXPORTER=none',
  ].join('\n')
}

/** A one-off curl that sends a single log, to check the key without the app. */
export function testLogCommand({ service, endpoint }: { service: string; endpoint: string }): string {
  return [
    `curl -X POST ${endpoint}/api/v1/telemetry/logs \\`,
    "  -H 'X-API-Key: PASTE_YOUR_API_KEY' \\",
    "  -H 'Content-Type: application/json' \\",
    `  -d '{"service_name":"${service}","severity":"INFO","message":"hello from AIDevOps"}'`,
  ].join('\n')
}
