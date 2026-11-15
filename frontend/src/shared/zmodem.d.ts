declare module 'zmodem.js-ex' {
  export interface Transfer {
    get_offset(): number
    send(bytes: Uint8Array): void
    end(bytes?: Uint8Array): Promise<void>
  }
  export interface Offer {
    get_details(): { name: string; size: number }
    accept(options: { on_input: (bytes: number[]) => void }): Promise<unknown>
    skip(): void
  }
  export interface Session {
    type: 'send' | 'receive'
    on(event: 'offer', callback: (offer: Offer) => void): void
    on(event: 'session_end', callback: () => void): void
    start(): Promise<void>
    send_offer(details: {
      name: string
      size: number
      mtime: Date
      files_remaining: number
      bytes_remaining: number
    }): Promise<Transfer | undefined>
    close(): Promise<void>
    abort(): void
  }
  export interface Detection {
    confirm(): Session
    deny(): void
  }
  class Sentry {
    constructor(options: {
      to_terminal: (bytes: number[]) => void
      sender: (bytes: number[]) => void
      on_detect: (detection: Detection) => void
      on_retract: () => void
    })
    consume(bytes: Uint8Array): void
    get_confirmed_session(): Session | undefined
  }
  const Zmodem: { Sentry: typeof Sentry }
  export default Zmodem
}
