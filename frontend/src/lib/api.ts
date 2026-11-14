export interface Conversation { id: string; title: string; model: string; created_at: string; updated_at: string }
export interface Message { id: number; role: string; content: string; created_at?: string }
export interface Action { tool: string; path?: string; command?: string; content?: string }
export interface RunEvent { approval_id?: string; message_id?: number; type: string; tool?: string; step?: number; output?: string; error?: string; response?: string; delta?: string; run_id?: string; conversation_id?: string; action?: Action }
export interface Run { id: string; conversation_id: string; status: string; state: { approval_id?: string; user_message_id?: number; prompt: string; events: RunEvent[]; pending?: Action; error?: string } }
export interface Detail { conversation: Conversation; messages: Message[]; runs: Run[] }
export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
 const response = await fetch('/api' + path, { ...options, headers: { 'Content-Type': 'application/json', ...options.headers } })
 if (!response.ok) { const body = await response.text(); try { throw new Error(JSON.parse(body).error || body) } catch (error) { if (error instanceof SyntaxError) throw new Error(body || response.statusText); throw error } }
 if (response.status === 204) return undefined as T
 return response.json()
}
export async function consumeSSE(body: ReadableStream<Uint8Array>, onEvent: (event: RunEvent) => void) {
 const reader = body.getReader(); const decoder = new TextDecoder(); let buffer = ''; let terminal = false
 function accept(frame: string) {
  const data = frame.split('\n').filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n')
  if (!data) return
  const event = JSON.parse(data) as RunEvent
  onEvent(event)
  if (['done', 'error', 'approval_required'].includes(event.type)) terminal = true
 }
 try {
  for (;;) {
   const { value, done } = await reader.read()
   buffer = (buffer + decoder.decode(value, { stream: !done })).replace(/\r\n/g, '\n')
   let index: number
   while ((index = buffer.indexOf('\n\n')) !== -1) { accept(buffer.slice(0, index)); buffer = buffer.slice(index + 2) }
   if (done) { if (buffer.trim()) accept(buffer); break }
  }
  if (!terminal) throw new Error('The stream ended before a final response. Reload the conversation to check its state.')
 } finally { reader.releaseLock() }
}
export async function stream(path: string, payload: unknown, signal: AbortSignal, onEvent: (event: RunEvent) => void) {
 const response = await fetch('/api' + path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload), signal })
 if (!response.ok) { const body = await response.text(); let message = body; try { message = JSON.parse(body).error || body } catch { /* plain HTTP error */ } throw new Error(message) }
 if (!response.body) throw new Error('Streaming is unavailable in this browser.')
 await consumeSSE(response.body, onEvent)
}
