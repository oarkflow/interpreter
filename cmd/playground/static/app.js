const editorRoot = document.getElementById('editor');
const editorCodeWrap = document.getElementById('editorCodeWrap');
const runBtn = document.getElementById('runBtn');
const formatBtn = document.getElementById('formatBtn');
const copyBtn = document.getElementById('copyBtn');
const resetBtn = document.getElementById('resetBtn');
const themeBtn = document.getElementById('themeBtn');
const authSecret = document.getElementById('authSecret');
const loginBtn = document.getElementById('loginBtn');
const logoutBtn = document.getElementById('logoutBtn');
const authStatus = document.getElementById('authStatus');
const authPanel = document.getElementById('authPanel');
const resultEl = document.getElementById('result');
const outputEl = document.getElementById('output');
const errorEl = document.getElementById('error');
const diagnosticsEl = document.getElementById('diagnostics');
const previewEl = document.getElementById('preview');
const durationEl = document.getElementById('duration');
const resultTypeEl = document.getElementById('resultType');
const outputLinesEl = document.getElementById('outputLines');
const statusBadge = document.getElementById('statusBadge');
const healthText = document.getElementById('healthText');
const exampleList = document.getElementById('exampleList');
const exampleSearch = document.getElementById('exampleSearch');
const exampleSearchWrap = document.getElementById('exampleSearchWrap');
const sidebarExamplesBtn = document.getElementById('sidebarExamplesBtn');
const sidebarGuideBtn = document.getElementById('sidebarGuideBtn');
const guideIntro = document.getElementById('guideIntro');
const guidePanel = document.getElementById('guidePanel');
const tabButtons = Array.from(document.querySelectorAll('.tab-btn'));
const panels = Array.from(document.querySelectorAll('.panel'));

const STORAGE_KEY = 'spl.playground.editor';
const THEME_KEY = 'spl.playground.theme';
const SIDEBAR_KEY = 'spl.playground.sidebar';

let codeExamples = {};
let monacoEditor = null;
let authenticated = false;
let currentSidebar = 'examples';

const interpreterGuideSections = [
  {
    title: 'What SPL Is',
    accent: 'from-slate-900 to-slate-700 dark:from-slate-100 dark:to-slate-300',
    body: 'SPL is a Go-based interpreter with script execution, a production-oriented playground, an interactive REPL, a module system, and a large builtin library for strings, collections, time, crypto, file IO, networking, database access, and embedding.',
    bullets: [
      'Primary entry points: `cmd/interpreter` for CLI and REPL, `cmd/playground` for the browser playground, and Go embedding APIs such as `Exec`, `ExecFile`, and `ExecWithOptions`.',
      'Scripts support variables, constants, functions, closures, control flow, arrays, hashes, methods, modules, and structured error handling with `try`, `catch`, and `throw`.',
      'The template engine is available as a separate module (`github.com/oarkflow/template`) with its own playground.'
    ]
  },
  {
    title: 'Execution Pipeline',
    accent: 'from-cyan-600 to-blue-600 dark:from-cyan-400 dark:to-blue-400',
    body: 'Playground execution follows a strict request-to-result pipeline so browser-submitted code is validated, sandboxed, evaluated, and serialized in a predictable way.',
    bullets: [
      'The browser posts JSON to `/api/execute` for SPL code. Unknown fields and non-JSON payloads are rejected before evaluation starts.',
      'The server resolves the working directory, then calls `interpreter.EvalForPlayground(...)` with bounded runtime options: depth, step count, heap, timeout, and module directory.',
      '`EvalForPlayground` builds a lexer, parser, and global environment, captures printed output into a buffer, evaluates the parsed program, and returns typed result metadata plus diagnostics.'
    ]
  },
  {
    title: 'Sandbox And Safety',
    accent: 'from-emerald-600 to-teal-600 dark:from-emerald-400 dark:to-teal-400',
    body: 'Interpreter safety is built around runtime limits and policy-based controls. The playground defaults are intentionally conservative because all code comes from the browser.',
    bullets: [
      'Playground evaluation enables `ProtectHost: true`, which prevents browser-submitted code from mutating the host process or filesystem.',
      'Runtime guards cap recursion depth, evaluation steps, heap usage, and wall-clock time to keep runaway code from consuming the server.',
      'The general sandbox system also supports strict-mode policies for exec, file access, environment writes, network targets, database drivers, and DSN patterns.',
      'HTTP handlers add rate limiting, session auth, panic recovery, body size caps, and security headers before requests reach the interpreter.'
    ]
  },
  {
    title: 'REPL Developer Experience',
    accent: 'from-violet-600 to-indigo-600 dark:from-violet-400 dark:to-indigo-400',
    body: 'The terminal REPL is designed as a modern interactive environment rather than a basic read-eval-print loop.',
    bullets: [
      'Interactive editing includes arrow-key history, persistent history files, semantic tab completion, inline suggestions, call tips, reverse history search, and parser-aware multiline input.',
      'Meta commands include `:help`, `:builtins`, `:search`, `:history`, `:vars`, `:type`, `:doc`, `:methods`, `:fields`, `:ast`, `:time`, `:debug`, `:mem`, `:load`, `:reload`, `:install`, `:config`, shell escapes, and `:reset`.',
      'Pretty-printed values, enhanced runtime errors, object introspection, AST inspection, memory usage, and timed evaluation are aimed at interpreter and language-tooling workflows.',
      'Secure config helpers mask secrets by default, so credentials loaded from `.env`, JSON, or YAML stay redacted in REPL output.'
    ]
  },
  {
    title: 'Playground Server Surface',
    accent: 'from-rose-600 to-pink-600 dark:from-rose-400 dark:to-pink-400',
    body: 'The browser UI is backed by a small HTTP API dedicated to health checks, auth state, examples, evaluation, and Prometheus-style metrics.',
    bullets: [
      'Status endpoints: `GET /api/health`, `GET /api/ready`, and `GET /api/session`.',
      'Auth endpoints: `POST /api/login` and `POST /api/logout`, with cookie-backed sessions when a playground secret is configured.',
      'Content endpoints: `GET /api/examples` and `POST /api/execute`.',
      'Observability endpoint: `GET /metrics` exposes request counters, auth events, active session counts, rate-limited requests, and execution duration histograms.'
    ]
  },
  {
    title: 'Operational Limits',
    accent: 'from-fuchsia-600 to-sky-600 dark:from-fuchsia-400 dark:to-sky-400',
    body: 'Most playground behavior is controlled with environment variables so the same binary can run locally or in hardened production deployments.',
    bullets: [
      'Server knobs include address binding, request body size, cookie security, proxy trust, read/write/idle timeouts, shutdown timeout, session TTL, and rate-limit windows.',
      'Interpreter knobs include `PLAYGROUND_EVAL_MAX_DEPTH`, `PLAYGROUND_EVAL_MAX_STEPS`, `PLAYGROUND_EVAL_MAX_HEAP_MB`, and `PLAYGROUND_EVAL_TIMEOUT_MS`.',
      'The CLI and embedded interpreter also support broader runtime controls such as `SPL_MAX_RECURSION`, `SPL_MAX_STEPS`, `SPL_EVAL_TIMEOUT_MS`, `SPL_MAX_HEAP_MB`, and `SPL_MODULE_PATH`.',
      'This makes the playground a thin, controlled surface over the same interpreter core used by scripts, tests, and embedding.'
    ]
  }
];

function escapeHTML(value) {
  return String(value || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function renderGuide() {
  if (!guidePanel) return;
  guidePanel.innerHTML = interpreterGuideSections.map((section) => `
    <section class="rounded-xl border border-slate-200 dark:border-slate-800 bg-white/70 dark:bg-slate-950/30 p-3 mb-3">
      <div class="inline-flex items-center rounded-full bg-gradient-to-r ${section.accent} px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.22em] text-white dark:text-slate-950">${escapeHTML(section.title)}</div>
      <p class="mt-3 text-sm leading-6 text-slate-700 dark:text-slate-200">${escapeHTML(section.body)}</p>
      <div class="mt-3 space-y-2">${section.bullets.map((item) => `
        <div class="rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50/90 dark:bg-slate-900/70 px-3 py-2 text-xs leading-5 text-slate-700 dark:text-slate-200">${escapeHTML(item)}</div>
      `).join('')}</div>
    </section>
  `).join('');
}

function setSidebarMode(mode) {
  currentSidebar = mode;
  localStorage.setItem(SIDEBAR_KEY, mode);
  const showingGuide = mode === 'guide';
  exampleList.classList.toggle('hidden', showingGuide);
  exampleSearchWrap.classList.toggle('hidden', showingGuide);
  guideIntro.classList.toggle('hidden', !showingGuide);
  guidePanel.classList.toggle('hidden', !showingGuide);
  sidebarExamplesBtn.className = showingGuide
    ? 'px-2.5 py-1.5 hover:bg-slate-100 dark:hover:bg-slate-800'
    : 'px-2.5 py-1.5 bg-slate-200 dark:bg-slate-800';
  sidebarGuideBtn.className = showingGuide
    ? 'px-2.5 py-1.5 bg-slate-200 dark:bg-slate-800'
    : 'px-2.5 py-1.5 hover:bg-slate-100 dark:hover:bg-slate-800';
}

// --- Editor helpers ---

function getEditorValue() {
  return monacoEditor ? monacoEditor.getValue() : '';
}

function setEditorValue(value) {
  if (monacoEditor) monacoEditor.setValue(value || '');
}

// --- UI state ---

function setBusy(isBusy) {
  runBtn.disabled = isBusy;
  runBtn.classList.toggle('opacity-70', isBusy);
  runBtn.textContent = isBusy ? 'Running...' : 'Run';
}

function setAuthState(isAuthed, text) {
  authenticated = isAuthed;
  runBtn.disabled = !authenticated;
  loginBtn.classList.toggle('hidden', authenticated);
  logoutBtn.classList.toggle('hidden', !authenticated);
  authSecret.disabled = authenticated;
  authStatus.textContent = text || (authenticated ? 'Signed in' : 'Signed out');
  if (!authenticated) {
    setStatus('idle', 'Sign in');
  }
}

function setStatus(kind, text) {
  const base = 'ml-1 inline-flex items-center px-2 py-0.5 rounded-full text-[11px]';
  if (kind === 'success') {
    statusBadge.className = `${base} bg-emerald-100 text-emerald-700`;
  } else if (kind === 'error') {
    statusBadge.className = `${base} bg-rose-100 text-rose-700`;
  } else if (kind === 'running') {
    statusBadge.className = `${base} bg-amber-100 text-amber-700`;
  } else {
    statusBadge.className = `${base} bg-slate-200 text-slate-700`;
  }
  statusBadge.textContent = text;
}

function setTab(tab) {
  for (const btn of tabButtons) {
    if (btn.dataset.tab === tab) {
      btn.classList.add('bg-slate-200', 'dark:bg-slate-800');
    } else {
      btn.classList.remove('bg-slate-200', 'dark:bg-slate-800');
    }
  }
  for (const panel of panels) {
    panel.classList.toggle('hidden', panel.dataset.panel !== tab);
  }
}

// --- Persistence ---

function persistCode() {
  localStorage.setItem(STORAGE_KEY, getEditorValue());
}

// --- Output ---

function updateOutputLines() {
  const text = outputEl.textContent || '';
  const lines = text.trim() ? text.split('\n').length : 0;
  outputLinesEl.textContent = String(lines);
}

function applyResponse(payload) {
  resultEl.textContent = payload.result || '-';
  outputEl.textContent = payload.output || '';
  const err = payload.error || '';
  errorEl.textContent = err ? `ERROR:\n${err}` : '';
  const diagnostics = Array.isArray(payload.diagnostics) ? payload.diagnostics : [];
  diagnosticsEl.textContent = diagnostics.length ? diagnostics.map((d, i) => `${i + 1}. ${d}`).join('\n\n') : '';
  durationEl.textContent = payload.duration_ms != null ? `${payload.duration_ms} ms` : '-';
  resultTypeEl.textContent = payload.result_type || '-';
  updateOutputLines();
  previewEl.srcdoc = '';

  // Detect HTML in result or output and render in preview iframe
  const htmlContent = detectHTML(payload.result) || detectHTML(payload.output);

  if (err) {
    const kind = payload.error_kind || 'error';
    setStatus('error', kind === 'parser' ? 'Parser Error' : 'Runtime Error');
    setTab('error');
  } else if (htmlContent) {
    previewEl.srcdoc = htmlContent;
    setStatus('success', 'Success');
    setTab('preview');
  } else if (payload.output) {
    setStatus('success', 'Success');
    setTab('output');
  } else {
    setStatus('success', 'Success');
    setTab('result');
  }
}

function detectHTML(text) {
  if (!text || typeof text !== 'string') return null;
  const trimmed = text.trim();
  if (/^<!DOCTYPE\s+html/i.test(trimmed) || /^<html[\s>]/i.test(trimmed)) return trimmed;
  if (/<(div|span|p|h[1-6]|table|form|section|article|main|header|footer|nav|ul|ol|button|input|select|style|script|link|meta)[\s>\/]/i.test(trimmed) && /<\/\w+>/.test(trimmed)) return trimmed;
  return null;
}

// --- Execution ---

async function runCode() {
  setBusy(true);
  setStatus('running', 'Running');
  errorEl.textContent = '';
  diagnosticsEl.textContent = '';
  try {
    const res = await fetch('/api/execute', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code: getEditorValue() }),
      credentials: 'include',
    });
    const payload = await res.json();
    if (res.status === 401) {
      setAuthState(false, 'Sign in required');
      errorEl.textContent = payload.error || 'Authentication required';
      diagnosticsEl.textContent = 'Use the Sign In control to create a server session.';
      setStatus('error', 'Auth Required');
      setTab('error');
      return;
    }
    applyResponse(payload);
  } catch (err) {
    errorEl.textContent = `Request failed: ${err.message}`;
    diagnosticsEl.textContent = 'Network/transport failure.';
    setStatus('error', 'Request Error');
    setTab('error');
  } finally {
    setBusy(false);
  }
}

function normalizeEditorInput(src) {
  const lines = src.split('\n');
  const cleaned = [];
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) {
      cleaned.push('');
      continue;
    }
    if (/^(go\s+run|go\s+test|npm\s+|bun\s+|node\s+)/.test(trimmed)) {
      continue;
    }
    cleaned.push(line);
  }
  return cleaned.join('\n').trim();
}

function formatCode() {
  const lines = getEditorValue().split('\n');
  let indent = 0;
  const out = [];
  for (const raw of lines) {
    const line = raw.trim();
    if (!line) {
      out.push('');
      continue;
    }
    if (line.startsWith('}') || line.startsWith('};')) {
      indent = Math.max(0, indent - 1);
    }
    out.push(`${'  '.repeat(indent)}${line}`);
    if (line.endsWith('{')) {
      indent += 1;
    }
  }
  setEditorValue(out.join('\n'));
  persistCode();
}

function clearPanels() {
  resultEl.textContent = '-';
  outputEl.textContent = '';
  errorEl.textContent = '';
  diagnosticsEl.textContent = '';
  previewEl.srcdoc = '';
  durationEl.textContent = '-';
  resultTypeEl.textContent = '-';
  updateOutputLines();
  setStatus('idle', 'Idle');
  setTab('result');
}

// --- Theme ---

function applyTheme(theme) {
  if (theme === 'dark') {
    document.documentElement.classList.add('dark');
    if (window.monaco) {
      monaco.editor.setTheme('vs-dark');
    }
  } else {
    document.documentElement.classList.remove('dark');
    if (window.monaco) {
      monaco.editor.setTheme('vs');
    }
  }
  localStorage.setItem(THEME_KEY, theme);
}

function initTheme() {
  const saved = localStorage.getItem(THEME_KEY);
  if (saved) {
    applyTheme(saved);
    return;
  }
  const preferredDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
  applyTheme(preferredDark ? 'dark' : 'light');
}

// --- Examples ---

function renderExamples(filter = '') {
  exampleList.innerHTML = '';
  const query = filter.trim().toLowerCase();
  const keys = Object.keys(codeExamples).filter((k) => k.toLowerCase().includes(query));
  for (const key of keys) {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'w-full text-left px-2.5 py-2 text-sm rounded-md border border-transparent hover:border-slate-300 dark:hover:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800';
    btn.textContent = key;
    btn.addEventListener('click', () => {
      setEditorValue(codeExamples[key] || '');
      persistCode();
      clearPanels();
    });
    exampleList.appendChild(btn);
  }
}

// --- Health & Examples loading ---

async function loadHealth() {
  try {
    const res = await fetch('/api/health', { credentials: 'include' });
    const payload = await res.json();
    healthText.textContent = payload.ok ? 'Service healthy' : 'Service unhealthy';
  } catch {
    healthText.textContent = 'Service unavailable';
  }
}

async function loadSession() {
  try {
    const res = await fetch('/api/session', { credentials: 'include' });
    const payload = await res.json();
    if (payload.auth_enabled === false) {
      authPanel.classList.add('hidden');
      authStatus.classList.add('hidden');
      setAuthState(true, '');
      return true;
    }
    if (payload.authenticated) {
      setAuthState(true, 'Signed in');
      return true;
    }
    setAuthState(false, 'Signed out');
    return false;
  } catch {
    setAuthState(false, 'Session unavailable');
    return false;
  }
}

async function login() {
  const secret = (authSecret.value || '').trim();
  if (!secret) {
    setAuthState(false, 'Enter the playground secret');
    return;
  }
  try {
    const res = await fetch('/api/login', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ secret }),
    });
    const payload = await res.json();
    if (!res.ok) {
      setAuthState(false, payload.error || 'Sign in failed');
      return;
    }
    authSecret.value = '';
    setAuthState(true, 'Signed in');
  } catch (err) {
    setAuthState(false, `Sign in failed: ${err.message}`);
  }
}

async function logout() {
  try {
    await fetch('/api/logout', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
    });
  } finally {
    setAuthState(false, 'Signed out');
  }
}

async function loadExamples() {
  try {
    const res = await fetch('/api/examples', { credentials: 'include' });
    const payload = await res.json();
    codeExamples = payload.examples || {};
    renderExamples('');

    if (!restoreCode()) {
      const first = Object.keys(codeExamples)[0];
      if (first) {
        setEditorValue(codeExamples[first]);
        persistCode();
      }
    }
  } catch (err) {
    errorEl.textContent = `Failed to load examples: ${err.message}`;
    setTab('error');
  }
}

function restoreCode() {
  const saved = localStorage.getItem(STORAGE_KEY);
  if (saved && saved.trim()) {
    setEditorValue(saved);
    return true;
  }
  return false;
}

// --- Monaco initialization ---

function initMonaco() {
  return new Promise((resolve, reject) => {
    if (!window.require) {
      reject(new Error('Monaco loader not found'));
      return;
    }
    window.require.config({ paths: { vs: 'https://cdnjs.cloudflare.com/ajax/libs/monaco-editor/0.52.2/min/vs' } });
    window.require(['vs/editor/editor.main'], () => {
      // Register SPL language
      monaco.languages.register({ id: 'spl' });
      monaco.languages.setMonarchTokensProvider('spl', {
        tokenizer: {
          root: [
            [/(let|const|if|else|while|for|break|continue|function|return|import|export|try|catch|throw|print|true|false|null)\b/, 'keyword'],
            [/\b[0-9]+\b/, 'number'],
            [/"([^"\\]|\\.)*"/, 'string'],
            [/\/\/.*$/, 'comment'],
            [/[a-zA-Z_][\w]*/, 'identifier'],
          ],
        },
      });

      const isDark = document.documentElement.classList.contains('dark');
      const editorTheme = isDark ? 'vs-dark' : 'vs';

      monacoEditor = monaco.editor.create(editorRoot, {
        automaticLayout: true,
        fontSize: 14,
        fontFamily: 'JetBrains Mono, Fira Code, Menlo, monospace',
        minimap: { enabled: false },
        roundedSelection: true,
        scrollBeyondLastLine: false,
        padding: { top: 12, bottom: 12 },
        theme: editorTheme,
        value: '',
        language: 'spl',
      });
      monacoEditor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => runBtn.click());
      monacoEditor.onDidChangeModelContent(() => persistCode());

      resolve();
    });
  });
}

// --- Event listeners ---

runBtn.addEventListener('click', async () => {
  if (!authenticated) {
    errorEl.textContent = 'Authentication required';
    diagnosticsEl.textContent = 'Use the Sign In control to create a session.';
    setStatus('error', 'Auth Required');
    setTab('error');
    return;
  }
  setEditorValue(normalizeEditorInput(getEditorValue()));
  persistCode();
  await runCode();
});

formatBtn.addEventListener('click', formatCode);

copyBtn.addEventListener('click', async () => {
  await navigator.clipboard.writeText(getEditorValue());
  setStatus('success', 'Copied');
});

resetBtn.addEventListener('click', () => {
  localStorage.removeItem(STORAGE_KEY);
  setEditorValue('');
  clearPanels();
});

themeBtn.addEventListener('click', () => {
  const next = document.documentElement.classList.contains('dark') ? 'light' : 'dark';
  applyTheme(next);
});

loginBtn.addEventListener('click', async () => {
  await login();
  if (authenticated) {
    await loadExamples();
  }
});

logoutBtn.addEventListener('click', async () => {
  await logout();
});

sidebarExamplesBtn.addEventListener('click', () => setSidebarMode('examples'));
sidebarGuideBtn.addEventListener('click', () => setSidebarMode('guide'));

exampleSearch.addEventListener('input', () => renderExamples(exampleSearch.value));

for (const btn of tabButtons) {
  btn.addEventListener('click', () => setTab(btn.dataset.tab));
}

// --- Boot ---

async function boot() {
  initTheme();
  const savedSidebar = localStorage.getItem(SIDEBAR_KEY);
  if (savedSidebar === 'guide') currentSidebar = 'guide';
  setAuthState(false, 'Signed out');
  setStatus('idle', 'Idle');
  setTab('result');
  clearPanels();
  renderGuide();
  try {
    await initMonaco();
  } catch (e) {
    errorEl.textContent = `Failed to initialize Monaco editor: ${e.message}`;
    setTab('error');
    return;
  }
  setSidebarMode(currentSidebar);
  await loadHealth();
  await loadSession();
  await loadExamples();
}

boot();
