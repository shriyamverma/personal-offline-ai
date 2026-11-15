import { describe, expect, it } from 'vitest'
import { consumeSSE, type RunEvent } from './api'
function chunks(parts: string[]) { const bytes = new TextEncoder().encode(parts.join('')); return new ReadableStream<Uint8Array>({ start(controller) { for (let i = 0; i < bytes.length; i++) controller.enqueue(bytes.slice(i, i + 1)); controller.close() } }) }
describe('SSE transport', () => {
 it('decodes split UTF-8, CRLF boundaries, and multiple events', async () => { const events: RunEvent[] = []; await consumeSSE(chunks(['data: {"type":"token","delta":"नमस्ते"}\r\n\r\n', 'data: {"type":"done","response":"नमस्ते"}\r\n\r\n']), event => events.push(event)); expect(events.map(event => event.type)).toEqual(['token', 'done']); expect(events[0].delta).toBe('नमस्ते') })
 it('accepts an error as a terminal event', async () => { const events: RunEvent[] = []; await consumeSSE(chunks(['data: {"type":"error","error":"Model unavailable"}\n\n']), event => events.push(event)); expect(events[0].error).toBe('Model unavailable') })
 it('reports an incomplete stream', async () => { await expect(consumeSSE(chunks(['data: {"type":"token","delta":"partial"}\n\n']), () => {})).rejects.toThrow('stream ended') })
})
