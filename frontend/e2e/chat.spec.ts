import { expect, test, type Page } from '@playwright/test'
import type { Conversation, Detail, RunEvent } from '../src/lib/api'
async function backend(page: Page) {
 let sequence = 0; let executions = 0
 const data = new Map<string, Detail>()
 await page.route('**/api/**', async route => {
  const path = new URL(route.request().url()).pathname.replace('/api', '')
  const method = route.request().method(); const body = route.request().postDataJSON() || {}
  const json = (value: unknown, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(value) })
  if (path === '/') return json({ status: 'ok' })
  if (path === '/models') return json({ models: ['qwen2.5:3b', 'llama3.2:3b'], default: 'qwen2.5:3b' })
  if (path === '/conversations' && method === 'GET') return json([...data.values()].map(item => item.conversation))
  if (path === '/conversations' && method === 'POST') {
   const conversation: Conversation = { id: String(++sequence), title: 'New conversation', model: body.model, created_at: '', updated_at: '' }
   data.set(conversation.id, { conversation, messages: [], runs: [] }); return json(conversation, 201)
  }
  if (path.startsWith('/conversations/')) {
   const id = path.split('/')[2]; const detail = data.get(id)!
   if (method === 'GET') return json(detail)
   if (method === 'DELETE') { data.delete(id); return route.fulfill({ status: 204 }) }
   Object.assign(detail.conversation, body); return json(detail.conversation)
  }
  let events: RunEvent[] = []
  if (path === '/chat') {
   const detail = data.get(body.conversation_id)!; const id = String(++sequence)
   const message = { id: ++sequence, role: 'user', content: body.prompt }; detail.messages.push(message)
   detail.conversation.title = body.prompt
   const pending = body.prompt.includes('pwd')
   const action = { tool: 'shell', command: 'pwd' }
   events = [{ type: 'run_start', run_id: id, message_id: message.id }, ...(pending ? [{ type: 'approval_required', run_id: id, approval_id: 'approval-' + id, action }] : [{ type: 'token', delta: '## Local answer\n\n```go\nfmt.Println("hello")\n```\n', run_id: id }, { type: 'done', response: '## Local answer\n\n```go\nfmt.Println("hello")\n```\n', run_id: id }])]
   detail.runs.push({ id, conversation_id: detail.conversation.id, status: pending ? 'awaiting_approval' : 'completed', state: { approval_id: 'approval-' + id, prompt: body.prompt, user_message_id: message.id, events, pending: pending ? action : undefined } })
   if (!pending) detail.messages.push({ id: ++sequence, role: 'assistant', content: events[2].response! })
  } else if (path.startsWith('/approvals/')) {
   const id = path.split('/')[2]; const detail = [...data.values()].find(item => item.runs.some(run => run.id === id))!; const run = detail.runs.find(run => run.id === id)!
   if (run.status !== 'awaiting_approval' || body.approval_id !== run.state.approval_id) return json({ error: 'already handled' }, 409)
   if (body.approved) executions++
   events = [{ type: 'run_start', run_id: id, message_id: run.state.user_message_id }, ...(body.approved ? [{ type: 'tool_start', tool: 'shell', run_id: id, step: 1, action: run.state.pending }, { type: 'tool_result', tool: 'shell', run_id: id, step: 1, output: '/workspace' }] : []), { type: 'done', run_id: id, response: body.approved ? 'Executed reviewed command.' : 'Action rejected.' }]
   run.status = body.approved ? 'completed' : 'rejected'; run.state.pending = undefined; run.state.events.push(...events); detail.messages.push({ id: ++sequence, role: 'assistant', content: events.at(-1)!.response! })
  } else return json({ error: 'unknown route: ' + path }, 404)
  return route.fulfill({ contentType: 'text/event-stream', body: events.map(event => 'data: ' + JSON.stringify(event) + '\n\n').join('') })
 })
 return { executions: () => executions }
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
 await page.getByLabel('Chat model', { exact: true }).selectOption('llama3.2:3b')
 await expect(page.getByLabel('Chat model', { exact: true })).toHaveValue('llama3.2:3b')
 await page.getByRole('button', { name: 'Rename Explain this code' }).click(); await page.getByLabel('Title', { exact: true }).fill('My saved chat'); await page.getByRole('button', { name: 'Save title' }).click()
 await expect(page.getByRole('button', { name: 'My saved chat', exact: true })).toBeVisible()
 await page.getByRole('button', { name: 'Toggle color theme' }).click(); const dark = await page.locator('html').evaluate(el => el.classList.contains('dark')); await page.reload(); await expect(page.locator('html')).toHaveClass(dark ? 'dark' : '')
 await page.getByRole('button', { name: 'Delete My saved chat' }).click(); await page.getByRole('button', { name: 'Delete conversation', exact: true }).click()
 await expect(page.getByRole('heading', { name: /What’s on your mind/ })).toBeVisible(); expect(errors).toEqual([])
})
test('approval survives reload and approves exactly once; rejection executes nothing', async ({ page }) => {
 const mock = await backend(page); await page.goto('/')
 await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Run pwd'); await page.getByRole('button', { name: 'Send message', exact: true }).click()
 await expect(page.getByRole('heading', { name: 'Your approval is needed' })).toBeVisible(); await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toBeDisabled()
 await page.reload(); await expect(page.getByRole('heading', { name: 'Your approval is needed' })).toBeVisible()
 await page.getByRole('button', { name: 'Approve & continue' }).click(); await expect(page.getByText('Executed reviewed command.', { exact: true })).toBeVisible(); expect(mock.executions()).toBe(1)
 await page.locator('summary').filter({ hasText: 'shell' }).click(); await expect(page.getByText('/workspace', { exact: true })).toBeVisible()
 await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Run pwd again'); await page.getByRole('button', { name: 'Send message', exact: true }).click(); await page.getByRole('button', { name: 'Reject', exact: true }).click()
 await expect(page.getByText('Action rejected.', { exact: true })).toBeVisible(); expect(mock.executions()).toBe(1)
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
