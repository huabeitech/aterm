/** The worker has a clock but no DOM timers. The host enforces each process timeout. */
export interface DockerDeadline {
  throwIfAborted(): void
}

export interface DockerBudget extends DockerDeadline {
  remaining(): number
}

export function dockerDeadline(milliseconds: number): DockerBudget {
  const deadline = Date.now() + milliseconds
  function remaining() {
    const time = deadline - Date.now()
    if (time <= 0) throw Error('Docker operation timed out')
    return Math.max(100, time)
  }
  return {
    remaining,
    throwIfAborted() { remaining() }
  }
}
