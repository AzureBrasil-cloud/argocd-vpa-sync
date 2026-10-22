// What the API client is doing right now, by stage, for the loading screen's
// progress bar. Each GET reports when it is sent, when its response headers
// arrive and when its body has been read; nothing here guesses at bytes.

export type LoadStage = 'request' | 'response' | 'done'

type Listener = () => void

const active = new Map<number, LoadStage>()
const listeners = new Set<Listener>()
let nextId = 1

function emit() {
  for (const listener of listeners) listener()
}

export function beginLoad(): number {
  const id = nextId++
  active.set(id, 'request')
  emit()
  return id
}

export function markLoad(id: number, stage: LoadStage): void {
  if (stage === 'done') active.delete(id)
  else active.set(id, stage)
  emit()
}

/** The least advanced stage among requests in flight ('done' when none are). */
export function currentLoadStage(): LoadStage {
  let stage: LoadStage = 'done'
  for (const s of active.values()) {
    if (s === 'request') return 'request'
    stage = 'response'
  }
  return stage
}

export function subscribeLoad(listener: Listener): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}
