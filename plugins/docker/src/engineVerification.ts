/** Read sampling shares a short verification lease; writes never use the cache. */
export class EngineVerification {
  private verified = new Map<string, number>()
  constructor(
    private now: () => number = Date.now,
    private ttl = 15000
  ) {}
  private key(token: string, context: string, engine: string) {
    return JSON.stringify([token, context, engine])
  }
  fresh(token: string, context: string, engine: string) {
    return (this.verified.get(this.key(token, context, engine)) ?? 0) > this.now()
  }
  remember(token: string, context: string, engine: string) {
    if (this.verified.size >= 64) this.verified.clear()
    this.verified.set(this.key(token, context, engine), this.now() + this.ttl)
  }
}
