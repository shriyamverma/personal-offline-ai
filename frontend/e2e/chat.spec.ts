import { expect, test, type Page } from '@playwright/test'
import type { Conversation, Detail, RunEvent } from '../src/lib/api'
async function backend(page: Page) {
 let sequence = 0
 const data = new Map<string, Detail>()
 await page.route('**/api/**', async route => {
  const path = new URL(route.request().url()).pathname.replace('/api', '')
  const method = route.request().method(); const body = route.request().postDataJSON() || {}
  const json = (value: unknown, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(value) })
  if (path === '/') return json({ status: 'ok' })
  if (path === '/models') return json({ models: ['openai/gpt-oss-20b', 'openai/gpt-oss-120b'], default: 'openai/gpt-oss-20b' })
  if (path === '/conversations' && method === 'GET') return json([...data.values()].map(item => item.conversation))
  if (path === '/conversations' && method === 'POST') {
   const conversation: Conversation = { id: String(++sequence), title: 'New conversation', model: body.model, created_at: '', updated_at: '' }
   data.set(conversation.id, { conversation, messages: [], runs: [] }); return json(conversation, 201)
  }
  if (path.startsWith('/conversations/')) {
   const id = path.split('/')[2]; const detail = data.get(id)!
   if (!detail) return json({ error: 'Conversation not found.' }, 404)
   if (method === 'GET') return json(detail)
   if (method === 'DELETE') { data.delete(id); return route.fulfill({ status: 204 }) }
   Object.assign(detail.conversation, body); return json(detail.conversation)
  }
  if (path !== '/chat') return json({ error: 'unknown route: ' + path }, 404)
  const detail = data.get(body.conversation_id)!; const id = String(++sequence)
  const message = { id: ++sequence, role: 'user', content: body.prompt }; detail.messages.push(message)
  detail.conversation.title = body.prompt
  const answer = '## Local answer\n\n```go\nfmt.Println("hello")\n```\n'
  const events: RunEvent[] = [{ type: 'run_start', run_id: id, message_id: message.id }, { type: 'token', delta: answer, run_id: id }, { type: 'done', response: answer, run_id: id }]
  detail.runs.push({ id, conversation_id: detail.conversation.id, status: 'completed', state: { prompt: body.prompt, user_message_id: message.id, events } })
  detail.messages.push({ id: ++sequence, role: 'assistant', content: answer })
  return route.fulfill({ contentType: 'text/event-stream', body: events.map(event => 'data: ' + JSON.stringify(event) + '\n\n').join('') })
 })
}
test('chat, markdown, history, models, rename, delete, and theme persist', async ({ page }) => {
 await backend(page); const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
 await page.goto('/'); await expect(page.getByRole('heading', { name: /What’s on your mind/ })).toBeVisible()
 await page.screenshot({ path: '/tmp/local-ai-desktop.png', fullPage: true })
 await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Explain this code')
 await page.getByRole('button', { name: 'Send message', exact: true }).click()
 await expect(page.getByRole('heading', { name: 'Local answer' })).toBeVisible()
 await expect(page.getByRole('button', { name: 'Copy code' })).toBeVisible()
 await page.reload(); await expect(page.getByRole('heading', { name: 'Local answer' })).toBeVisible()
 await page.getByLabel('Chat model', { exact: true }).selectOption('openai/gpt-oss-120b')
 await expect(page.getByLabel('Chat model', { exact: true })).toHaveValue('openai/gpt-oss-120b')
 await page.getByRole('button', { name: 'Rename Explain this code' }).click(); await page.getByLabel('Title', { exact: true }).fill('My saved chat'); await page.getByRole('button', { name: 'Save title' }).click()
 await expect(page.getByRole('button', { name: 'My saved chat', exact: true })).toBeVisible()
 await page.getByRole('button', { name: 'Toggle color theme' }).click(); const dark = await page.locator('html').evaluate(el => el.classList.contains('dark')); await page.reload(); await expect(page.locator('html')).toHaveClass(dark ? 'dark' : '')
 await page.getByRole('button', { name: 'Delete My saved chat' }).click(); await page.getByRole('button', { name: 'Delete conversation', exact: true }).click()
 await expect(page.getByRole('heading', { name: /What’s on your mind/ })).toBeVisible(); expect(errors).toEqual([])
})
test('mobile layout and settings remain usable', async ({ page }) => {
 await page.setViewportSize({ width: 390, height: 844 }); await backend(page); await page.goto('/')
 await page.getByRole('button', { name: 'Open sidebar' }).click(); await page.getByRole('button', { name: /Settings/ }).click(); await expect(page.getByRole('dialog')).toBeVisible()
 await page.getByLabel('Appearance', { exact: true }).selectOption('dark'); await page.getByRole('button', { name: 'Close dialog' }).click(); await page.getByRole('button', { name: 'Close sidebar' }).click({ position: { x: 380, y: 400 } })
 await expect(page.locator('.sidebar')).not.toBeInViewport()
 await page.screenshot({ path: '/tmp/local-ai-mobile.png', fullPage: true })
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
 await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toBeVisible()
})

test('independent light and dark palettes persist and follow system changes', async ({ page }) => {
 await backend(page)
 await page.emulateMedia({ colorScheme: 'light' })
 await page.goto('/')
 await page.getByRole('button', { name: /Settings/ }).click()
 const light = page.getByRole('group', { name: 'Default light theme', exact: true })
 const dark = page.getByRole('group', { name: 'Default dark theme', exact: true })
 await light.getByText('Paper & Ink', { exact: true }).click()
 await dark.getByText('Warm Graphite', { exact: true }).click()
 await expect(page.locator('html')).toHaveAttribute('data-palette', 'paper')
 await expect(page.locator('html')).not.toHaveClass('dark')
 // The inactive dark choice must not change the current light palette.
 await expect(light.getByRole('radio', { name: 'Paper & Ink', exact: true })).toBeChecked()
 await page.emulateMedia({ colorScheme: 'dark' })
 await expect(page.locator('html')).toHaveAttribute('data-palette', 'graphite')
 await expect(page.locator('html')).toHaveClass('dark')
 await expect(page.getByRole('status')).toHaveText('Currently using Warm Graphite · Dark')
 await page.reload()
 await expect(page.locator('html')).toHaveAttribute('data-palette', 'graphite')
 await page.getByRole('button', { name: /Settings/ }).click()
 await expect(light.getByRole('radio', { name: 'Paper & Ink', exact: true })).toBeChecked()
 await expect(dark.getByRole('radio', { name: 'Warm Graphite', exact: true })).toBeChecked()
 await page.getByLabel('Appearance', { exact: true }).selectOption('light')
 await expect(page.locator('html')).toHaveAttribute('data-palette', 'paper')
 await page.emulateMedia({ colorScheme: 'dark' })
 await expect(page.locator('html')).toHaveAttribute('data-palette', 'paper')
 await page.getByLabel('Appearance', { exact: true }).selectOption('dark')
 await expect(page.locator('html')).toHaveAttribute('data-palette', 'graphite')
 await page.getByRole('button', { name: 'Close dialog' }).click()
 await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Theme validation')
 const send = page.getByRole('button', { name: 'Send message', exact: true })
 await expect(send).toHaveCSS('background-color', 'rgb(230, 185, 107)')
 await expect(send).toHaveCSS('color', 'rgb(43, 33, 18)')
 await page.getByRole('button', { name: 'Toggle color theme' }).click()
 await expect(page.locator('html')).toHaveAttribute('data-palette', 'paper')
 await expect(page.locator('html')).not.toHaveClass('dark')
})


test('legacy Ollama preference migrates to the cloud default without losing appearance', async ({ page }) => {
 await page.addInitScript(() => localStorage.setItem('local-ai.preferences', JSON.stringify({ theme: 'dark', lightTheme: 'nord', darkTheme: 'graphite', model: 'qwen2.5:3b', enterToSend: false })))
 await backend(page)
 await page.goto('/')
 await expect(page.getByLabel('Chat model', { exact: true })).toHaveValue('openai/gpt-oss-20b')
 await expect(page.locator('html')).toHaveClass('dark')
 await expect(page.locator('html')).toHaveAttribute('data-palette', 'graphite')
 await page.getByRole('button', { name: /Settings/ }).click()
 await expect(page.getByLabel('Default model for new chats', { exact: true })).toHaveValue('openai/gpt-oss-20b')
 await expect(page.getByRole('checkbox', { name: 'Press Enter to send messages' })).not.toBeChecked()
})


test('missing saved conversation recovers to a new chat after changing databases', async ({ page }) => {
 await backend(page)
 await page.goto('/')
 await page.evaluate(() => localStorage.setItem('local-ai.active', JSON.stringify('old-offline-conversation')))
 await page.reload()
 await expect.poll(() => page.evaluate(() => JSON.parse(localStorage.getItem('local-ai.active') || 'null'))).toBe(null)
 await expect(page.getByRole('heading', { name: /What’s on your mind/ })).toBeVisible()
 await expect(page.getByRole('alert')).toHaveCount(0)
 await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Hello in the cloud')
 await page.getByRole('button', { name: 'Send message', exact: true }).click()
 await expect(page.getByRole('heading', { name: 'Local answer' })).toBeVisible()
})

test('temporary backend failure does not discard the saved conversation', async ({ page }) => {
 await backend(page)
 await page.route('**/api/conversations/unavailable-chat', route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Temporarily unavailable' }) }))
 await page.goto('/')
 await page.evaluate(() => localStorage.setItem('local-ai.active', JSON.stringify('unavailable-chat')))
 await page.reload()
 await expect(page.getByRole('alert')).toContainText('Temporarily unavailable')
 expect(await page.evaluate(() => JSON.parse(localStorage.getItem('local-ai.active') || 'null'))).toBe('unavailable-chat')
})


test('chat-only interface offers chat prompts and handles command requests as ordinary messages', async ({ page }) => {
 await backend(page)
 const paths: string[] = []
 page.on('request', request => { if (request.url().includes('/api/')) paths.push(new URL(request.url()).pathname) })
 await page.goto('/')
 await expect(page.getByText('Chat-only mode.', { exact: false })).toBeVisible()
 await expect(page.getByRole('button', { name: /Explain a concept/ })).toBeVisible()
 await expect(page.getByRole('button', { name: /Explore a codebase/ })).toHaveCount(0)
 await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Run pwd')
 await page.getByRole('button', { name: 'Send message', exact: true }).click()
 await expect(page.getByRole('heading', { name: 'Local answer' })).toBeVisible()
 await expect(page.getByRole('button', { name: /Approve|Reject/ })).toHaveCount(0)
 await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toBeEnabled()
 await page.reload()
 await expect(page.getByRole('heading', { name: 'Local answer' })).toBeVisible()
 expect(paths.some(path => /approvals|capabilities/.test(path))).toBe(false)
})
