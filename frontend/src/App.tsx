import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import {
  ArrowUp,
  Check,
  ChevronDown,
  Code2,
  Copy,
  FileText,
  ListTodo,
  HardDrive,
  Loader2,
  Menu,
  MessageSquare,
  Moon,
  Pencil,
  Plus,
  Search,
  Settings,
  ShieldCheck,
  Square,
  Sun,
  Trash2,
  X,
} from "lucide-react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Button } from "./components/ui/button";
import { ThemePicker } from "./components/theme-picker";
import { normalizePreferences, type Preferences } from "./lib/preferences";
import { getPalette } from "./lib/themes";
import { useTheme } from "./lib/use-theme";
import {
  api,
  ApiError,
  stream,
  type Conversation,
  type Detail,
  type Run,
  type RunEvent,
} from "./lib/api";

function stored<T>(key: string, fallback: T): T {
  try {
    const value = localStorage.getItem(key);
    return value ? (JSON.parse(value) as T) : fallback;
  } catch {
    return fallback;
  }
}
function Code({ children, className, ...props }: React.ComponentProps<"code">) {
  const [copied, setCopied] = useState(false);
  const isBlock = !!className || String(children).includes("\n");
  if (!isBlock)
    return (
      <code className={className} {...props}>
        {children}
      </code>
    );
  return (
    <span className="code-block">
      <span className="code-toolbar">
        <span>{className?.replace("language-", "") || "text"}</span>
        <button
          type="button"
          aria-label="Copy code"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(String(children));
              setCopied(true);
              setTimeout(() => setCopied(false), 2000);
            } catch {
              setCopied(false);
            }
          }}
        >
          {copied ? <Check size={14} /> : <Copy size={14} />}
          {copied ? "Copied" : "Copy"}
        </button>
      </span>
      <code className={className} {...props}>
        {children}
      </code>
    </span>
  );
}
function Prose({ content }: { content: string }) {
  return (
    <div className="prose">
      <Markdown
        remarkPlugins={[remarkGfm]}
        components={{
          code: Code,
          a: ({ children, ...props }) => (
            <a {...props} target="_blank" rel="noreferrer">
              {children}
            </a>
          ),
          img: ({ alt }) => (
            <span className="muted">
              [Image: {alt || "external image hidden"}]
            </span>
          ),
        }}
      >
        {content}
      </Markdown>
    </div>
  );
}
function Modal({
  title,
  children,
  onClose,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const dialog = ref.current;
    dialog?.showModal();
    return () => dialog?.close();
  }, []);
  return (
    <dialog
      ref={ref}
      aria-label={title}
      onCancel={onClose}
      onClick={(event) => {
        if (event.target === ref.current) onClose();
      }}
    >
      <div className="modal-title">
        <h2>{title}</h2>
        <Button
          variant="ghost"
          size="icon"
          aria-label="Close dialog"
          onClick={onClose}
        >
          <X size={18} />
        </Button>
      </div>
      {children}
    </dialog>
  );
}
function ResponseStatus({ run }: { run: Run }) {
  const memoryWarning = (run.state.events || []).find(event => event.type === "memory_warning");
  return (
    <div className="response-status" aria-label="Response status">
      {memoryWarning && <p className="muted" role="status">{memoryWarning.error}</p>}
      {run.status === "running" && <div className="thinking"><Loader2 size={14} className="spin" /> Preparing a response…</div>}
      {["failed", "interrupted"].includes(run.status) && <p className="run-error">{run.state.error || "This request was interrupted. You can send a new message."}</p>}
    </div>
  );
}
export default function App() {
  const [preferences, setPreferences] = useState<Preferences>(() =>
    normalizePreferences(stored("local-ai.preferences", {})),
  );
  const resolvedMode = useTheme(
    preferences.theme,
    preferences.lightTheme,
    preferences.darkTheme,
  );
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [activeId, setActiveId] = useState<string | null>(() =>
    stored("local-ai.active", null),
  );
  const [detail, setDetail] = useState<Detail | null>(null);
  const [models, setModels] = useState<string[]>([]);
  const [modelError, setModelError] = useState("");
  const [query, setQuery] = useState("");
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [online, setOnline] = useState(false);
  const [settings, setSettings] = useState(false);
  const [sidebar, setSidebar] = useState(false);
  const [rename, setRename] = useState<Conversation | null>(null);
  const [renameTitle, setRenameTitle] = useState("");
  const [remove, setRemove] = useState<Conversation | null>(null);
  const [partial, setPartial] = useState("");
  const controller = useRef<AbortController | null>(null);
  const end = useRef<HTMLDivElement>(null);
  const skipInitialLoad = useRef<string | null>(null);
  const activeRef = useRef(activeId);
  const requestVersion = useRef(0);
  const textarea = useRef<HTMLTextAreaElement>(null);
  activeRef.current = activeId;
  const report = (err: unknown) =>
    setError(err instanceof Error ? err.message : String(err));
  useEffect(() => {
    localStorage.setItem("local-ai.preferences", JSON.stringify(preferences));
  }, [preferences]);
  async function refreshList() {
    const list = await api<Conversation[]>("/conversations");
    setConversations(list);
    return list;
  }
  async function refreshModels() {
    try {
      const result = await api<{ models: string[]; default?: string }>("/models");
      setModels(result.models);
      if (result.models.length) {
        setPreferences(value => result.models.includes(value.model) ? value : {
          ...value, model: result.default && result.models.includes(result.default) ? result.default : result.models[0],
        });
      }
      setModelError("");
    } catch (err) {
      setModelError(err instanceof Error ? err.message : String(err));
    }
  }
  useEffect(() => {
    let live = true;
    api<Conversation[]>("/conversations")
      .then((list) => {
        if (live) {
          setConversations(list);
          setOnline(true);
        }
      })
      .catch((err) => {
        if (live) report(err);
      });
    void refreshModels();
    const timer = setInterval(() => {
      api("/")
        .then(() => setOnline(true))
        .catch(() => setOnline(false));
    }, 15000);
    return () => {
      live = false;
      clearInterval(timer);
      controller.current?.abort();
    };
  }, []);
  useEffect(() => {
    localStorage.setItem("local-ai.active", JSON.stringify(activeId));
    if (skipInitialLoad.current === activeId && activeId) {
      skipInitialLoad.current = null;
      return;
    }
    setError("");
    setDetail(null);
    setPartial("");
    const version = ++requestVersion.current;
    if (!activeId) {
      setLoading(false);
      return;
    }
    setLoading(true);
    api<Detail>("/conversations/" + activeId)
      .then((result) => {
        if (requestVersion.current === version) setDetail(result);
      })
      .catch((err) => {
        if (requestVersion.current !== version) return;
        if (err instanceof ApiError && err.status === 404) {
          // A branch switch, database reset, or deletion in another tab can
          // leave a saved conversation ID that this backend no longer has.
          activeRef.current = null;
          setActiveId(null);
          setDetail(null);
          setConversations(items => items.filter(item => item.id !== activeId));
        } else report(err);
      })
      .finally(() => {
        if (requestVersion.current === version) setLoading(false);
      });
  }, [activeId]);
  useEffect(() => {
    end.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [detail?.messages.length, detail?.runs, partial]);
  useEffect(() => {
    if (
      !activeId ||
      busy ||
      !detail?.runs.some((run) => run.status === "running")
    )
      return;
    const id = activeId;
    const timer = setInterval(() => {
      api<Detail>("/conversations/" + id)
        .then((value) => {
          if (activeRef.current === id) setDetail(value);
        })
        .catch(report);
    }, 2000);
    return () => clearInterval(timer);
  }, [activeId, busy, detail?.runs]);
  const runningElsewhere =
    detail?.runs.some((run) => run.status === "running") && !busy;
  const selectedModel = detail?.conversation.model || preferences.model;
  function newChat() {
    if (busy) return;
    setActiveId(null);
    setDetail(null);
    setDraft("");
    setError("");
    setSidebar(false);
    textarea.current?.focus();
  }
  function receive(event: RunEvent) {
    if (event.type === "error") {
      setError(event.error || "Request failed");
      return;
    }
    if (event.type === "token") {
      setPartial((value) => value + (event.delta || ""));
      return;
    }
    setDetail((value) => {
      if (!value || !event.run_id) return value;
      const runs = [...value.runs];
      const index = runs.findIndex((run) => run.id === event.run_id);
      const run: Run =
        index >= 0
          ? {
              ...runs[index],
              state: {
                ...runs[index].state,
                events: [...runs[index].state.events],
              },
            }
          : {
              id: event.run_id,
              conversation_id: value.conversation.id,
              status: "running",
              state: { prompt: "", events: [] },
            };
      run.state.events.push(event);
      if (event.type === "run_start") {
        run.status = "running";
        run.state.user_message_id = event.message_id;
      }
      if (event.type === "done") run.status = "completed";
      if (index >= 0) runs[index] = run;
      else runs.push(run);
      const messages =
        event.type === "run_start" && event.message_id
          ? value.messages.map((message) =>
              message.id < 0 ? { ...message, id: event.message_id! } : message,
            )
          : value.messages;
      return { ...value, messages, runs };
    });
  }
  async function runRequest(path: string, payload: unknown, id: string) {
    controller.current = new AbortController();
    setPartial("");
    try {
      await stream(path, payload, controller.current.signal, receive);
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError")
        setError("Response stopped. You can send another message.");
      else report(err);
    } finally {
      controller.current = null;
      try {
        const result = await api<Detail>("/conversations/" + id);
        if (activeRef.current === id) {
          setDetail(result);
          setPartial("");
        }
        await refreshList();
      } catch (err) {
        report(err);
      }
      setBusy(false);
    }
  }
  async function send(event?: FormEvent, promptOverride?: string) {
    event?.preventDefault();
    const prompt = (promptOverride || draft).trim();
    if (!prompt || busy || loading || runningElsewhere) return;
    setBusy(true);
    setError("");
    setSidebar(false);
    try {
      let id = activeId;
      if (!id) {
        const c = await api<Conversation>("/conversations", {
          method: "POST",
          body: JSON.stringify({ model: preferences.model }),
        });
        id = c.id;
        activeRef.current = id;
        skipInitialLoad.current = id;
        setActiveId(id);
        // Finish initial detail loading before applying streaming events.
        ++requestVersion.current;
        setDetail({ conversation: c, messages: [], runs: [] });
        setLoading(false);
      }
      setDraft("");
      setDetail((value) =>
        value
          ? {
              ...value,
              messages: [
                ...value.messages,
                { id: -Date.now(), role: "user", content: prompt },
              ],
            }
          : value,
      );
      await runRequest(
        "/chat",
        { conversation_id: id, prompt, stream: true },
        id,
      );
    } catch (err) {
      report(err);
      setBusy(false);
    }
  }
  async function changeModel(model: string) {
    if (busy || runningElsewhere) return;
    try {
      if (activeId && detail) {
        const c = await api<Conversation>("/conversations/" + activeId, {
          method: "PATCH",
          body: JSON.stringify({ model }),
        });
        setDetail({ ...detail, conversation: c });
        await refreshList();
      } else setPreferences((value) => ({ ...value, model }));
    } catch (err) {
      report(err);
    }
  }
  const allModels = [...new Set([selectedModel, ...models])];
  return (
    <div className="app-shell">
      {sidebar && (
        <button
          className="sidebar-overlay"
          aria-label="Close sidebar"
          onClick={() => setSidebar(false)}
        />
      )}
      <aside className={`sidebar ${sidebar ? "is-open" : ""}`}>
        <div className="brand">
          <span className="brand-mark">
            <HardDrive size={20} />
          </span>
          <span className="brand-name">
            Shriyam <span className="brand-dot">AI</span>
          </span>
        </div>
        <p className="workspace-owner">Shriyam’s workspace</p>
        <Button
          className="new-chat"
          variant="outline"
          onClick={newChat}
          disabled={busy}
        >
          <Plus size={16} /> New conversation <kbd>＋</kbd>
        </Button>
        <label className="search">
          <Search size={15} />
          <input
            placeholder="Search conversations"
            aria-label="Search conversations"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        <div className="section-label">
          YOUR CONVERSATIONS <span>{conversations.length}</span>
        </div>
        <nav className="conversation-list" aria-label="Conversations">
          {conversations
            .filter((c) => c.title.toLowerCase().includes(query.toLowerCase()))
            .map((c) => (
              <div
                className={`conversation-row ${activeId === c.id ? "selected" : ""}`}
                key={c.id}
              >
                <button
                  className="conversation-link"
                  disabled={busy}
                  onClick={() => {
                    setActiveId(c.id);
                    setSidebar(false);
                  }}
                >
                  <MessageSquare size={15} />
                  <span>{c.title}</span>
                </button>
                <div className="row-actions">
                  <button
                    aria-label={`Rename ${c.title}`}
                    disabled={busy}
                    onClick={() => {
                      setRename(c);
                      setRenameTitle(c.title);
                    }}
                  >
                    <Pencil size={12} />
                  </button>
                  <button
                    aria-label={`Delete ${c.title}`}
                    disabled={busy}
                    onClick={() => setRemove(c)}
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
              </div>
            ))}
          {!conversations.length && (
            <p className="sidebar-empty">
              A little space for big ideas.
              <br />
              Your conversations will appear here.
            </p>
          )}
          {query &&
            !conversations.some((c) =>
              c.title.toLowerCase().includes(query.toLowerCase()),
            ) && <p className="sidebar-empty">No conversations found.</p>}
        </nav>
        <div className="sidebar-bottom">
          <div className="privacy-note">
            <ShieldCheck size={17} />
            <div>
              <strong>Yours, by design</strong>
              <p>Groq models. Cloud retrieval.</p>
            </div>
          </div>
          <button
            className="settings-button"
            onClick={() => {
              setSettings(true);
              void refreshModels();
            }}
          >
            <Settings size={16} /> Settings{" "}
            <span>
              <span className={`status-dot ${online ? "online" : ""}`} />
              {online ? "API connected" : "API offline"}
            </span>
          </button>
        </div>
      </aside>
      <main className="main-pane">
        <header className="topbar">
          <Button
            className="mobile-menu"
            variant="ghost"
            size="icon"
            aria-label="Open sidebar"
            onClick={() => setSidebar(true)}
          >
            <Menu size={20} />
          </Button>
          <div className="breadcrumb">
            Workspace <span>/</span>{" "}
            <strong>{detail?.conversation.title || "New conversation"}</strong>
          </div>
          <div className="local-badge">
            <span className="status-dot online" /> Cloud chat
          </div>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Toggle color theme"
            onClick={() =>
              setPreferences((value) => ({
                ...value,
                theme: resolvedMode === "dark" ? "light" : "dark",
              }))
            }
          >
            {resolvedMode === "dark" ? <Sun size={17} /> : <Moon size={17} />}
          </Button>
        </header>
        <div className="chat-scroll">
          {loading && (
            <div className="loading">
              <Loader2 className="spin" size={20} /> Loading conversation…
            </div>
          )}
          {!loading && !detail?.messages.length && (
            <section className="welcome">
              <div className="welcome-label">
                <span className="status-dot online" /> YOUR PRIVATE THINKING
                SPACE
              </div>
              <div className="welcome-symbol">
                <HardDrive size={33} strokeWidth={1.3} />
                <span>✦</span>
              </div>
              <h1>
                What’s on your mind,
                <br />
                Shriyam?
              </h1>
              <p>
                Think it through. Learn something. Make something new.
                <br />
                Your personal assistant, powered by Groq. Chat-only mode.
              </p>
              <div className="suggestions">
                {[
                  { icon: <Code2 size={19} />, title: "Explain a concept", text: "Explain how semantic search works with a simple example" },
                  { icon: <ListTodo size={19} />, title: "Plan a project", text: "Help me break a project idea into achievable milestones" },
                  { icon: <FileText size={19} />, title: "Draft something", text: "Help me draft a clear project introduction" },
                ].map((item) => (
                  <button
                    disabled={busy}
                    key={item.title}
                    onClick={() => void send(undefined, item.text)}
                  >
                    {item.icon}
                    <strong>{item.title}</strong>
                    <span>{item.text}</span>
                    <span className="suggestion-arrow">↗</span>
                  </button>
                ))}
              </div>
              <div className="welcome-foot">
                <ShieldCheck size={13} /> History is saved on your server. Cloud services process requests.
              </div>
            </section>
          )}
          {!!detail?.messages.length && (
            <div className="messages">
              {detail.messages.map((message) => (
                <div key={message.id}>
                  {message.role === "user" ? (
                    <article className="message user">
                      <span className="message-label">YOU</span>
                      <div>{message.content}</div>
                    </article>
                  ) : (
                    <article className="message assistant">
                      <span className="assistant-label">
                        <span className="mini-mark">✦</span> SHRIYAM AI
                      </span>
                      <Prose content={message.content} />
                    </article>
                  )}
                  {message.role === "user" &&
                    (() => {
                      const run = detail.runs.find(
                        (item) => item.state.user_message_id === message.id,
                      );
                      return run ? <ResponseStatus run={run} /> : null;
                    })()}
                </div>
              ))}
              {partial && (
                <article className="message assistant streaming">
                  <span className="assistant-label">
                    <span className="mini-mark">✦</span> SHRIYAM AI
                  </span>
                  <Prose content={partial} />
                </article>
              )}
              {busy &&
                !partial &&
                !detail.runs.some((r) => r.status === "running") && (
                  <div className="thinking">
                    <Loader2 className="spin" size={14} /> Connecting to your
                    cloud model…
                  </div>
                )}
              {runningElsewhere && (
                <p className="thinking">
                  <Loader2 size={14} className="spin" /> A request is running.
                  This conversation will update automatically.
                </p>
              )}
            </div>
          )}
          <div ref={end} />
        </div>
        <div className="composer-area">
          {error && (
            <div className="error-banner" role="alert">
              <span>{error}</span>
              <button aria-label="Dismiss error" onClick={() => setError("")}>
                <X size={16} />
              </button>
            </div>
          )}
          <form className="composer" onSubmit={(event) => void send(event)}>
            <textarea
              ref={textarea}
              aria-label="Message"
              placeholder="Ask anything…"
              value={draft}
              disabled={loading || !!runningElsewhere}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                if (
                  event.key === "Enter" &&
                  !event.shiftKey &&
                  preferences.enterToSend &&
                  !event.nativeEvent.isComposing
                ) {
                  event.preventDefault();
                  void send();
                }
              }}
              rows={2}
            />
            <div className="composer-toolbar">
              <label className="model-selector">
                <span className="status-dot online" />
                <select
                  aria-label="Chat model"
                  value={selectedModel}
                  disabled={busy || !!runningElsewhere || loading}
                  onChange={(event) => void changeModel(event.target.value)}
                >
                  {allModels.map((model) => (
                    <option key={model} value={model}>
                      {model}
                    </option>
                  ))}
                </select>
                <ChevronDown size={12} />
              </label>
              <span className="enter-hint">
                {busy
                  ? "Working…"
                  : preferences.enterToSend
                    ? "Enter to send"
                    : "Shift + Enter for a new line"}
              </span>
              {busy ? (
                <Button
                  type="button"
                  size="icon"
                  aria-label="Stop response"
                  disabled={!controller.current}
                  onClick={() => controller.current?.abort()}
                >
                  <Square size={14} fill="currentColor" />
                </Button>
              ) : (
                <Button
                  type="submit"
                  size="icon"
                  aria-label="Send message"
                  disabled={
                    !draft.trim() || loading || !!runningElsewhere
                  }
                >
                  <ArrowUp size={19} />
                </Button>
              )}
            </div>
          </form>
          <div className="composer-foot">
            <span>
              <ShieldCheck size={12} /> Powered by Groq
            </span>
            <span>AI can make mistakes. Review important results.</span>
          </div>
        </div>
      </main>
      {settings && (
        <Modal title="Assistant settings" onClose={() => setSettings(false)}>
          <p className="muted">
            Preferences are saved in this browser. History is saved on your server. Groq processes prompts; Qdrant processes memory text.
          </p>
          <label className="field">
            Appearance
            <select
              aria-label="Appearance"
              value={preferences.theme}
              onChange={(event) =>
                setPreferences((value) => ({
                  ...value,
                  theme: event.target.value as Preferences["theme"],
                }))
              }
            >
              <option value="system">Follow system</option>
              <option value="light">Light</option>
              <option value="dark">Dark</option>
            </select>
          </label>
          <div className="theme-settings">
            <p className="muted">
              Choose a palette for each mode. Follow system switches between
              your two defaults automatically.
            </p>
            <p className="theme-current" role="status">
              Currently using{" "}
              {
                getPalette(
                  resolvedMode === "dark"
                    ? preferences.darkTheme
                    : preferences.lightTheme,
                ).name
              }{" "}
              · {resolvedMode === "dark" ? "Dark" : "Light"}
            </p>
            <ThemePicker
              mode="light"
              value={preferences.lightTheme}
              onChange={(lightTheme) =>
                setPreferences((value) => ({ ...value, lightTheme }))
              }
            />
            <ThemePicker
              mode="dark"
              value={preferences.darkTheme}
              onChange={(darkTheme) =>
                setPreferences((value) => ({ ...value, darkTheme }))
              }
            />
            <p className="muted">
              Select Light or Dark above to preview that mode. Choices are saved
              automatically in this browser.
            </p>
          </div>
          <label className="field">
            Default model for new chats
            <select
              aria-label="Default model for new chats"
              value={preferences.model}
              onChange={(event) =>
                setPreferences((value) => ({
                  ...value,
                  model: event.target.value,
                }))
              }
            >
              {[...new Set([preferences.model, ...models])].map((model) => (
                <option key={model}>{model}</option>
              ))}
            </select>
          </label>
          <label className="checkbox-field">
            <input
              type="checkbox"
              checked={preferences.enterToSend}
              onChange={(event) =>
                setPreferences((value) => ({
                  ...value,
                  enterToSend: event.target.checked,
                }))
              }
            />{" "}
            Press Enter to send messages
          </label>
          <p className="muted">
            Shift + Enter always inserts a new line. Models must be enabled for your Groq account.
          </p>
          {modelError && <p className="run-error">{modelError}</p>}
          <Button variant="outline" onClick={() => void refreshModels()}>
            Refresh Groq models
          </Button>
          <footer className="creator-credit">
            Designed &amp; built by <strong>Shriyam Verma</strong>
          </footer>
        </Modal>
      )}
      {rename && (
        <Modal title="Rename conversation" onClose={() => setRename(null)}>
          <form
            onSubmit={async (event) => {
              event.preventDefault();
              try {
                const c = await api<Conversation>(
                  "/conversations/" + rename.id,
                  {
                    method: "PATCH",
                    body: JSON.stringify({ title: renameTitle.trim() }),
                  },
                );
                if (detail?.conversation.id === c.id)
                  setDetail({ ...detail, conversation: c });
                await refreshList();
                setRename(null);
              } catch (err) {
                report(err);
                setRename(null);
              }
            }}
          >
            <label className="field">
              Title
              <input
                autoFocus
                maxLength={240}
                value={renameTitle}
                onChange={(event) => setRenameTitle(event.target.value)}
              />
            </label>
            <Button type="submit" disabled={!renameTitle.trim()}>
              Save title
            </Button>
          </form>
        </Modal>
      )}
      {remove && (
        <Modal title="Delete conversation?" onClose={() => setRemove(null)}>
          <p className="muted">
            “{remove.title}” and its messages and summaries
            will be permanently deleted.
          </p>
          <div className="dialog-actions">
            <Button variant="outline" onClick={() => setRemove(null)}>
              Keep conversation
            </Button>
            <Button
              variant="destructive"
              onClick={async () => {
                try {
                  await api("/conversations/" + remove.id, {
                    method: "DELETE",
                  });
                  if (activeId === remove.id) newChat();
                  await refreshList();
                  setRemove(null);
                } catch (err) {
                  report(err);
                  setRemove(null);
                }
              }}
            >
              Delete conversation
            </Button>
          </div>
        </Modal>
      )}
    </div>
  );
}
