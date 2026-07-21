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
const artifactsEl = document.getElementById('artifacts');
const previewEl = document.getElementById('preview');
const durationEl = document.getElementById('duration');
const resultTypeEl = document.getElementById('resultType');
const outputLinesEl = document.getElementById('outputLines');
const statusBadge = document.getElementById('statusBadge');
const renderServerStatus = document.getElementById('renderServerStatus');
const renderModeEl = document.getElementById('renderMode');
const renderAllowUrlsEl = document.getElementById('renderAllowUrls');
const renderUrlHostsEl = document.getElementById('renderUrlHosts');
const healthText = document.getElementById('healthText');
const exampleList = document.getElementById('exampleList');
const exampleSearch = document.getElementById('exampleSearch');
const exampleSearchWrap = document.getElementById('exampleSearchWrap');
const tabButtons = Array.from(document.querySelectorAll('.tab-btn'));
const panels = Array.from(document.querySelectorAll('.panel'));

const STORAGE_KEY = 'spl.playground.editor';
const THEME_KEY = 'spl.playground.theme';
const RENDER_URLS_KEY = 'spl.playground.render_urls';

let codeExamples = {};
let monacoEditor = null;
let authenticated = false;
let serverRenderConfig = { mode: 'auto', max_bytes: 1048576, allow_urls: false, allow_url_hosts: [] };

// Groups the example/tool browser in the left sidebar. Every entry here is a
// runnable snippet: picking one loads it into the editor, Run executes it
// through the normal /api/execute pipeline. Categories with no matching
// example key (e.g. PDF/Database/XQL on the lightweight `playground`
// binary, which doesn't link those builtins) simply don't render.
const categoryOrder = [
  'Getting Started', 'Modules & Imports', 'Files & Renaming', 'Archives & Images', 'Media',
  'Secrets & Crypto', 'System & Network', 'Dates & Time', 'Daily Ops Utilities', 'PDF', 'Database', 'XQL',
  'Artifacts & Data', 'Servers & Runtime', 'Other',
];

const exampleCategories = {
  hello: 'Getting Started', functions: 'Getting Started', formatting: 'Getting Started',
  collections: 'Getting Started', 'pattern-matching': 'Getting Started', loops: 'Getting Started',
  math: 'Getting Started', strings: 'Getting Started', 'collections-advanced': 'Getting Started',
  'type-casting': 'Getting Started', crypto: 'Getting Started', time: 'Getting Started',
  testing: 'Getting Started', 'complete-tour': 'Getting Started',
  modules: 'Modules & Imports', 'std-modules': 'Modules & Imports', 'package-imports': 'Modules & Imports',
  'tools-files': 'Files & Renaming',
  'tools-images': 'Archives & Images', 'image-values': 'Archives & Images',
  'tools-media': 'Media',
  'tools-secrets': 'Secrets & Crypto',
  securetoken: 'Secrets & Crypto',
  shamir: 'Secrets & Crypto',
  'tools-network': 'System & Network',
  naturaldate: 'Dates & Time',
  wuid: 'Daily Ops Utilities',
  money: 'Daily Ops Utilities',
  phone: 'Daily Ops Utilities',
  metadata: 'Daily Ops Utilities',
  ip: 'System & Network',
  'pdf-tools': 'PDF',
  'query-builder': 'Database',
  'xql-basics': 'XQL',
  artifacts: 'Artifacts & Data', 'file-values': 'Artifacts & Data', 'json-csv-values': 'Artifacts & Data',
  'write-ops': 'Artifacts & Data', 'reactive-html': 'Artifacts & Data',
  'stateful-server': 'Servers & Runtime', 'server-middleware': 'Servers & Runtime',
  'reactive-state': 'Servers & Runtime', scheduler: 'Servers & Runtime',
  'runtime-session': 'Servers & Runtime', 'config-secrets': 'Servers & Runtime',
  'resource-limits': 'Servers & Runtime', 'production-profile': 'Servers & Runtime',
  'server-sse': 'Servers & Runtime', 'server-route-groups': 'Servers & Runtime',
};

const defaultOpenCategories = new Set(['Getting Started']);

function categoryFor(key) {
  return exampleCategories[key] || 'Other';
}

const splToolCompletions = [
  'bulk_rename', 'file_search', 'file_locate', 'file_finder', 'file_move_plan', 'file_copy_plan', 'file_dedupe',
  'archive_compress', 'archive_extract', 'archive_list',
  'image_convert_batch', 'image_optimize', 'image_crop_file',
  'office_text', 'secret_generate', 'token_generate', 'file_encrypt', 'file_decrypt',
  'media_info', 'media_convert', 'ffmpeg_status', 'ffmpeg_install',
  'system_info', 'dns_lookup', 'tcp_check', 'http_probe',
  'pdf_info', 'pdf_validate', 'pdf_merge', 'pdf_split', 'pdf_to_text', 'pdf_search',
  'db_connect', 'db_query', 'db_exec', 'query', 'lazy_query',
  'xql_run', 'xql_connect', 'xql_list_integrations',
  'naturaldate_parse', 'naturaldate_parse_all',
  'wuid_new', 'wuid_new_uuid', 'wuid_parse',
  'money_new', 'money_add', 'money_sub', 'money_mul', 'money_percent', 'money_format',
  'phone_parse', 'phone_valid', 'phone_country', 'phone_parse_bulk', 'phone_networks',
  'securetoken_encrypt', 'securetoken_decrypt',
  'ip_is_private', 'ip_client_from_header', 'ip_geo_init', 'ip_country', 'ip_lookup', 'ip_lookup_bulk',
  'shamir_split', 'shamir_combine',
  'infer_csv_types', 'infer_json_types', 'infer_value_type'
];

const splToolModules = [
  'tools/files', 'tools/archive', 'tools/images', 'tools/office',
  'tools/secrets', 'tools/media', 'tools/system', 'tools/network',
  'pdf', 'database', 'xql', 'naturaldate', 'wuid', 'money', 'phone', 'securetoken', 'ip', 'shamir', 'metadata'
];

const splToolHoverDocs = {
  bulk_rename: 'bulk_rename(dir[, opts]) previews or applies bulk file renames. Options include match, template, and apply.',
  file_search: 'file_search(root[, opts]) searches files or directories by glob, literal, or regex patterns plus name, path, extension, content, size, time, sort, and limit filters.',
  file_finder: 'file_finder(root) creates a chainable finder: .files().regex("^examples_.*\\\\.spl$").content_regex("print\\\\s+").limit(5).exec().',
  archive_compress: 'archive_compress(src, dst[, opts]) previews or creates zip, tar, or gzip archives.',
  image_convert_batch: 'image_convert_batch(src_dir, dst_dir[, opts]) previews or converts image files in bulk.',
  secret_generate: 'secret_generate([length][, alphabet]) returns a masked generated secret.',
  media_convert: 'media_convert(src, dst[, opts]) previews or converts media with ffmpeg.',
  ffmpeg_status: 'ffmpeg_status() reports ffmpeg and ffprobe availability.',
  ffmpeg_install: 'ffmpeg_install([opts]) previews or runs the detected OS ffmpeg installer.',
  pdf_info: 'pdf_info(path) returns page count, encryption state, and metadata for a PDF file.',
  pdf_merge: 'pdf_merge(dst, ...srcs) combines multiple PDF files into one (filesystem write, trusted profile only).',
  pdf_to_text: 'pdf_to_text(path) extracts plain text content from a PDF.',
  pdf_search: 'pdf_search(path, query[, opts]) searches PDF text content for a query string.',
  db_connect: 'db_connect(driver, dsn) opens a database connection, e.g. db_connect("sqlite", ":memory:").',
  db_query: 'db_query(db, sql[, args]) runs a SQL query and returns matching rows.',
  db_exec: 'db_exec(db, sql[, args]) executes a SQL statement (INSERT/UPDATE/DDL) and returns the result.',
  query: 'query(db, table) starts a chainable query builder: .select().where().order_by().limit().exec().',
  xql_run: 'xql_run(query) executes a federated XQL query, optionally joining across connected integrations.',
  xql_connect: 'xql_connect(alias, type, config[, source]) registers a data source (SQL, HTTP, file, ...) for use in XQL queries.',
  xql_list_integrations: 'xql_list_integrations() lists the data sources currently connected via xql_connect.',
  naturaldate_parse: 'naturaldate_parse(text[, opts]) returns (result, err) for a single natural-language date/time expression, e.g. "tomorrow at 9am" or "next monday".',
  naturaldate_parse_all: 'naturaldate_parse_all(text[, opts]) extracts every date/time expression embedded in free-form text.',
  wuid_new: 'wuid_new() generates a sortable, time-ordered unique ID as fixed-width base62 text.',
  wuid_new_uuid: 'wuid_new_uuid() generates the same sortable ID as wuid_new, formatted as a standard dashed UUID.',
  wuid_parse: 'wuid_parse(id) returns (result, err) with the embedded creation time of a base62/hex/UUID id string.',
  money_new: 'money_new(amount, currency_code) builds a fixed-point money value, e.g. money_new("19.99", "USD").',
  money_add: 'money_add(a, b) returns (money, err); both operands must share a currency.',
  money_percent: 'money_percent(money, pct) returns pct percent of the amount, e.g. money_percent(price, 8.5) for 8.5% tax.',
  money_format: 'money_format(money[, opts]) formats a money value for display, e.g. "$19.99".',
  phone_parse: 'phone_parse(number[, default_region]) returns (result, err) with e164/international/national formats, validity, type, and best-effort carrier/network (MCC/MNC/PLMN) info.',
  phone_valid: 'phone_valid(number[, default_region]) is a convenience check that never throws - just true/false.',
  phone_country: 'phone_country(country_code) looks up a 2-letter ISO country code\'s name, dialing prefix, and currency.',
  phone_parse_bulk: 'phone_parse_bulk(records[, field][, opts]) validates a phone field (or a bare array/slice of numbers) across many rows (ARRAY or TABLE_VALUE from JSON/CSV/DB/XQL), returning {total, valid_count, invalid_count, results}.',
  phone_networks: 'phone_networks(country_code[, opts]) lists known mobile network operators (MCC, MNC, PLMN, brand/operator, status) for a 2-letter ISO country code; opts.status filters e.g. to "Operational".',
  securetoken_encrypt: 'securetoken_encrypt(claims, secret[, opts]) encrypts a HASH of claims into an AES-256-GCM token.',
  securetoken_decrypt: 'securetoken_decrypt(token, secret[, opts]) decrypts and authenticates a token, throwing if the secret is wrong or the token was tampered with.',
  ip_is_private: 'ip_is_private(ip) reports whether an address is loopback, RFC1918/link-local IPv4, or unique-local/link-local IPv6.',
  ip_client_from_header: 'ip_client_from_header(remote_ip, header_value[, opts]) extracts the real client IP from a proxy-chain header, preferring the first public address.',
  ip_geo_init: 'ip_geo_init() downloads/loads a local IP geolocation dataset (network + filesystem-write capability required) so ip_country/ip_lookup can resolve real data.',
  ip_country: 'ip_country(ip) returns a 2-letter country code, or "" until ip_geo_init() has loaded the dataset.',
  ip_lookup: 'ip_lookup(ip) returns a HASH with country/region/city/lat/long; found=false until ip_geo_init() has loaded the dataset.',
  ip_lookup_bulk: 'ip_lookup_bulk(records, field[, opts]) validates and geolocates an IP field across many rows (ARRAY or TABLE_VALUE from JSON/CSV/DB/XQL), returning {total, valid_count, invalid_count, results}.',
  shamir_split: 'shamir_split(secret, threshold, shares[, auth_key]) splits a secret into N shares where any threshold of them reconstruct it, returning {shares, auth_key}.',
  shamir_combine: 'shamir_combine(shares, auth_key) reconstructs the secret from at least threshold shares and the matching auth_key from shamir_split.',
  infer_csv_types: 'infer_csv_types(csv_text) profiles raw CSV text, returning a HASH mapping each column to its inferred type (int, float64, bool, time.Time, string).',
  infer_json_types: 'infer_json_types(value) profiles an already-decoded JSON HASH or ARRAY of HASH rows, returning a per-field inferred-type HASH.',
  infer_value_type: 'infer_value_type(value) infers the type of a single value directly, e.g. infer_value_type("2026-01-01") -> "time.Time".'
};

function escapeHTML(value) {
  return String(value || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
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
  runBtn.textContent = isBusy ? 'Running…' : 'Run';
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
  const variant = { success: 'status--ok', error: 'status--err', running: 'status--busy' }[kind] || 'status--idle';
  statusBadge.className = `status ${variant}`;
  statusBadge.textContent = text;
}

function setTab(tab) {
  for (const btn of tabButtons) {
    btn.classList.toggle('is-active', btn.dataset.tab === tab);
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

function parseHostList(value) {
  return String(value || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}

function persistedRenderUrlsEnabled() {
  const raw = localStorage.getItem(RENDER_URLS_KEY);
  if (raw == null) return null;
  return raw === 'true';
}

function applyRenderConfig(config) {
  serverRenderConfig = {
    mode: config && config.mode ? config.mode : 'auto',
    max_bytes: config && config.max_bytes ? config.max_bytes : 1048576,
    allow_urls: !!(config && config.allow_urls),
    allow_url_hosts: Array.isArray(config && config.allow_url_hosts) ? config.allow_url_hosts : [],
  };
  if (renderModeEl) renderModeEl.value = serverRenderConfig.mode || 'auto';
  if (renderAllowUrlsEl) {
    const saved = persistedRenderUrlsEnabled();
    renderAllowUrlsEl.checked = serverRenderConfig.allow_urls && (saved == null ? true : saved);
    renderAllowUrlsEl.disabled = !serverRenderConfig.allow_urls;
    renderAllowUrlsEl.title = serverRenderConfig.allow_urls
      ? 'Allow this run to resolve URL artifacts through the playground server.'
      : 'Server URL rendering is disabled. Start with PLAYGROUND_RENDER_ALLOW_URLS=true.';
  }
  if (renderUrlHostsEl) {
    renderUrlHostsEl.value = serverRenderConfig.allow_url_hosts.join(',');
    renderUrlHostsEl.disabled = !serverRenderConfig.allow_urls;
    renderUrlHostsEl.title = serverRenderConfig.allow_urls
      ? 'Optional comma-separated subset of server-allowed URL hosts.'
      : 'Server URL rendering is disabled. Configure PLAYGROUND_RENDER_ALLOW_URL_HOSTS.';
  }
  if (renderServerStatus) {
    const hosts = serverRenderConfig.allow_url_hosts.length ? serverRenderConfig.allow_url_hosts.join(',') : 'any host';
    renderServerStatus.textContent = serverRenderConfig.allow_urls ? `URLs available: ${hosts}` : 'URLs disabled';
  }
}

function renderRequestOptions() {
  return {
    render_mode: renderModeEl ? renderModeEl.value : serverRenderConfig.mode,
    render_allow_urls: !!(renderAllowUrlsEl && renderAllowUrlsEl.checked && serverRenderConfig.allow_urls),
    render_url_hosts: parseHostList(renderUrlHostsEl ? renderUrlHostsEl.value : ''),
    render_max_bytes: serverRenderConfig.max_bytes,
  };
}

function renderArtifacts(artifacts) {
  const items = Array.isArray(artifacts) ? artifacts : [];
  if (!artifactsEl) return null;
  artifactsEl.innerHTML = '';
  if (!items.length) return null;

  let previewHTML = null;
  for (const item of items) {
    const mime = String(item.mime || '').toLowerCase();
    const name = item.name || item.source || item.kind || 'artifact';
    const wrap = document.createElement('section');
    wrap.className = 'card';
    const head = document.createElement('div');
    head.className = 'card__head';
    head.innerHTML = `<strong>${escapeHTML(name)}</strong><span class="card__meta">${escapeHTML(item.kind || 'file')}</span><span class="card__meta">${escapeHTML(item.mime || 'unknown')}</span><span class="card__meta">${item.size || 0} bytes</span>`;
    wrap.appendChild(head);

    const body = document.createElement('div');
    body.className = 'card__body';
    if (item.error) {
      body.className += ' card__body--error';
      body.textContent = item.error.includes('URL rendering is disabled')
        ? `${item.error}. Enable server URL rendering with PLAYGROUND_RENDER_ALLOW_URLS=true and optionally PLAYGROUND_RENDER_ALLOW_URL_HOSTS, then tick URLs before running.`
        : item.error;
    } else if (item.data_url && mime.startsWith('image/')) {
      const img = document.createElement('img');
      img.src = item.data_url;
      img.alt = item.alt || name;
      img.className = 'card__image';
      if (item.width) img.width = item.width;
      if (item.height) img.height = item.height;
      body.appendChild(img);
      if (!previewHTML) {
        previewHTML = `<!doctype html><html><body style="margin:0;min-height:100vh;display:grid;place-items:center;background:#fff"><img src="${item.data_url}" alt="${escapeHTML(item.alt || name)}" style="max-width:100%;max-height:100vh;object-fit:contain"></body></html>`;
      }
    } else if (item.content && mime.includes('html')) {
      const frame = document.createElement('iframe');
      frame.className = 'card__frame';
      frame.setAttribute('sandbox', 'allow-scripts');
      frame.srcdoc = item.content;
      body.appendChild(frame);
      if (!previewHTML) previewHTML = item.content;
    } else if (item.content) {
      const pre = document.createElement('pre');
      pre.className = 'card__pre';
      pre.textContent = item.content;
      body.appendChild(pre);
      if (!previewHTML) {
        previewHTML = `<pre style="white-space:pre-wrap;font:13px ui-monospace,SFMono-Regular,Menlo,monospace;padding:16px">${escapeHTML(item.content)}</pre>`;
      }
    } else {
      body.className += ' card__body--muted';
      body.textContent = `${item.source_type || 'source'} artifact available as ${item.mime || 'unknown type'}.`;
    }
    wrap.appendChild(body);
    artifactsEl.appendChild(wrap);
  }
  return previewHTML;
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
  const artifactPreview = renderArtifacts(payload.artifacts);

  // Detect HTML in result or output and render in preview iframe
  const htmlContent = artifactPreview || detectHTML(payload.result) || detectHTML(payload.output);

  if (err) {
    const kind = payload.error_kind || 'error';
    setStatus('error', kind === 'parser' ? 'Parser Error' : 'Runtime Error');
    setTab('error');
  } else if (htmlContent) {
    previewEl.srcdoc = htmlContent;
    setStatus('success', 'Success');
    setTab('preview');
  } else if (Array.isArray(payload.artifacts) && payload.artifacts.length) {
    setStatus('success', 'Success');
    setTab('artifacts');
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
      body: JSON.stringify({ code: getEditorValue(), ...renderRequestOptions() }),
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
  if (artifactsEl) artifactsEl.innerHTML = '';
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

  const groups = new Map();
  for (const key of Object.keys(codeExamples)) {
    if (query && !key.toLowerCase().includes(query)) continue;
    const cat = categoryFor(key);
    if (!groups.has(cat)) groups.set(cat, []);
    groups.get(cat).push(key);
  }

  const orderedCats = categoryOrder.filter((cat) => groups.has(cat));
  for (const cat of orderedCats) {
    const keys = groups.get(cat);
    const details = document.createElement('details');
    details.className = 'example-group';
    details.open = query.length > 0 || defaultOpenCategories.has(cat);

    const summary = document.createElement('summary');
    summary.className = 'example-group__title';
    summary.innerHTML = `<span>${escapeHTML(cat)}</span><span class="example-group__count">${keys.length}</span>`;
    details.appendChild(summary);

    const list = document.createElement('div');
    list.className = 'example-group__list';
    for (const key of keys) {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'list-item';
      btn.textContent = key;
      btn.addEventListener('click', () => {
        setEditorValue(codeExamples[key] || '');
        persistCode();
        clearPanels();
      });
      list.appendChild(btn);
    }
    details.appendChild(list);
    exampleList.appendChild(details);
  }
}

// --- Health & Examples loading ---

async function loadHealth() {
  const healthDot = document.getElementById('healthDot');
  try {
    const res = await fetch('/api/health', { credentials: 'include' });
    const payload = await res.json();
    healthText.textContent = payload.ok ? 'service healthy' : 'service unhealthy';
    if (healthDot) healthDot.className = `dot ${payload.ok ? 'dot--ok' : 'dot--err'}`;
  } catch {
    healthText.textContent = 'service unavailable';
    if (healthDot) healthDot.className = 'dot dot--err';
  }
}

async function loadSession() {
  try {
    const res = await fetch('/api/session', { credentials: 'include' });
    const payload = await res.json();
    applyRenderConfig(payload.render || {});
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
            [/\b(let|const|if|else|while|for|in|and|or|not|break|continue|function|return|import|export|try|catch|throw|print|true|false|null)\b/i, 'keyword'],
            [/\b[0-9]+\b/, 'number'],
            [/"([^"\\]|\\.)*"/, 'string'],
            [/\/\/.*$/, 'comment'],
            [/[a-zA-Z_][\w]*/, 'identifier'],
          ],
        },
      });

      monaco.languages.registerCompletionItemProvider('spl', {
        triggerCharacters: ['_', '"', '/'],
        provideCompletionItems(model, position) {
          const word = model.getWordUntilPosition(position);
          const range = {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: word.startColumn,
            endColumn: word.endColumn,
          };
          const builtinSuggestions = splToolCompletions.map((label) => ({
            label,
            kind: monaco.languages.CompletionItemKind.Function,
            detail: splToolHoverDocs[label] || 'SPL daily tools builtin',
            insertText: label,
            range,
          }));
          const moduleSuggestions = splToolModules.map((label) => ({
            label,
            kind: monaco.languages.CompletionItemKind.Module,
            detail: 'SPL daily tools module',
            insertText: label,
            range,
          }));
          return { suggestions: builtinSuggestions.concat(moduleSuggestions) };
        },
      });

      monaco.languages.registerHoverProvider('spl', {
        provideHover(model, position) {
          const word = model.getWordAtPosition(position);
          if (!word || !splToolHoverDocs[word.word]) return null;
          return {
            range: new monaco.Range(position.lineNumber, word.startColumn, position.lineNumber, word.endColumn),
            contents: [{ value: `**${word.word}**\n\n${splToolHoverDocs[word.word]}` }],
          };
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

if (renderAllowUrlsEl) {
  renderAllowUrlsEl.addEventListener('change', () => {
    localStorage.setItem(RENDER_URLS_KEY, String(renderAllowUrlsEl.checked));
  });
}

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

exampleSearch.addEventListener('input', () => renderExamples(exampleSearch.value));

for (const btn of tabButtons) {
  btn.addEventListener('click', () => setTab(btn.dataset.tab));
}

// --- Boot ---

async function boot() {
  initTheme();
  setAuthState(false, 'Signed out');
  setStatus('idle', 'Idle');
  setTab('result');
  clearPanels();
  try {
    await initMonaco();
  } catch (e) {
    errorEl.textContent = `Failed to initialize Monaco editor: ${e.message}`;
    setTab('error');
    return;
  }
  await loadHealth();
  await loadSession();
  await loadExamples();
}

boot();
