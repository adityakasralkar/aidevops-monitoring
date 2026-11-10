import type { ReactNode, SVGProps } from 'react'

type IconProps = SVGProps<SVGSVGElement> & { size?: number }

// Shared frame for all icons: 24-unit grid, stroke follows the text colour.
function Icon({ size = 16, children, ...rest }: IconProps & { children: ReactNode }) {
  return (
    <svg
      aria-hidden="true"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...rest}
    >
      {children}
    </svg>
  )
}

export function OverviewIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <rect x="3" y="3" width="7" height="7" rx="1.5" />
      <rect x="14" y="3" width="7" height="7" rx="1.5" />
      <rect x="3" y="14" width="7" height="7" rx="1.5" />
      <rect x="14" y="14" width="7" height="7" rx="1.5" />
    </Icon>
  )
}

export function LogsIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M4 6h16M4 12h16M4 18h10" />
    </Icon>
  )
}

export function ErrorsIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M12 3 2 20h20L12 3z" />
      <path d="M12 10v4M12 17v.5" />
    </Icon>
  )
}

export function MetricsIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <polyline points="3,17 9,11 13,15 21,7" />
    </Icon>
  )
}

export function PipelinesIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <circle cx="6" cy="6" r="2.5" />
      <circle cx="6" cy="18" r="2.5" />
      <circle cx="18" cy="9" r="2.5" />
      <path d="M6 8.5v7M18 11.5c0 3-3 4-9.5 5" />
    </Icon>
  )
}

export function ConnectIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M9 2v6M15 2v6M6 8h12v4a6 6 0 0 1-12 0V8zM12 18v4" />
    </Icon>
  )
}

export function SettingsIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9 7 7M17 17l2.1 2.1M4.9 19.1 7 17M17 7l2.1-2.1" />
    </Icon>
  )
}

export function LogOutIcon(props: IconProps) {
  return (
    <Icon strokeWidth={2} {...props}>
      <path d="M9 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h3" />
      <path d="M16 17l5-5-5-5" />
      <path d="M21 12H9" />
    </Icon>
  )
}

export function ChevronsUpDownIcon(props: IconProps) {
  return (
    <Icon strokeWidth={2} {...props}>
      <path d="m7 10 5-5 5 5M7 14l5 5 5-5" />
    </Icon>
  )
}

export function DiagnoseIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3.5-3.5M11 8v3l2 2" />
    </Icon>
  )
}

export function ServerIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <rect x="3" y="4" width="18" height="7" rx="2" />
      <rect x="3" y="13" width="18" height="7" rx="2" />
      <path d="M7 7.5h.01M7 16.5h.01" />
    </Icon>
  )
}

export function ClockIcon(props: IconProps) {
  return (
    <Icon strokeWidth={2} {...props}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </Icon>
  )
}

export function MenuIcon(props: IconProps) {
  return (
    <Icon strokeWidth={2} {...props}>
      <path d="M4 7h16M4 12h16M4 17h16" />
    </Icon>
  )
}

export function CloseIcon(props: IconProps) {
  return (
    <Icon strokeWidth={2} {...props}>
      <path d="M6 6l12 12M18 6 6 18" />
    </Icon>
  )
}

export function PlusIcon(props: IconProps) {
  return (
    <Icon strokeWidth={2} {...props}>
      <path d="M12 5v14M5 12h14" />
    </Icon>
  )
}

export function WarningIcon(props: IconProps) {
  return (
    <Icon strokeWidth={2} {...props}>
      <path d="M12 3 2 20h20L12 3z" />
      <path d="M12 10v4M12 17v.5" />
    </Icon>
  )
}

export function ExternalIcon(props: IconProps) {
  return (
    <Icon strokeWidth={2} {...props}>
      <path d="M7 17 17 7M8 7h9v9" />
    </Icon>
  )
}

export function CheckIcon(props: IconProps) {
  return (
    <Icon strokeWidth={2.2} {...props}>
      <path d="m5 12.5 4.5 4.5L19 7.5" />
    </Icon>
  )
}
