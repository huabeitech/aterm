import type { TunnelInput, TunnelStatus } from '../../../../shared/contracts/tunnels'

export function tunnelAddress(address: string, port: number): string {
  return `${address.includes(':') ? `[${address}]` : address}:${port}`
}

export function tunnelNeedsRestart(tunnel: TunnelStatus): boolean {
  const active = tunnel.activeConfig
  return (
    !!active &&
    (active.hostID !== tunnel.hostID ||
      active.localAddress !== tunnel.localAddress ||
      active.localPort !== tunnel.localPort ||
      active.targetAddress !== tunnel.targetAddress ||
      active.targetPort !== tunnel.targetPort)
  )
}

export function validateTunnel(input: TunnelInput): string | null {
  if (!input.name.trim()) return 'Enter a tunnel name.'
  if (new TextEncoder().encode(input.name.trim()).length > 200)
    return 'Use a tunnel name of up to 200 bytes.'
  if (!input.hostID) return 'Select an SSH host.'
  if (!input.localAddress.trim()) return 'Enter a local listen address.'
  if (
    !Number.isInteger(input.localPort) ||
    input.localPort < 1 ||
    input.localPort > 65535 ||
    !Number.isInteger(input.targetPort) ||
    input.targetPort < 1 ||
    input.targetPort > 65535
  )
    return 'Ports must be whole numbers between 1 and 65535.'
  if (!input.targetAddress.trim() || /[\s/\\\x00]/.test(input.targetAddress))
    return 'Enter a target hostname or IP address without a port or URL scheme.'
  return null
}

export function localAddressIsLoopback(address: string): boolean {
  return address === '::1' || /^127\.(\d{1,3}\.){2}\d{1,3}$/.test(address)
}
