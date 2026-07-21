// Projects mode: a multi-file SPL project IDE backed by /api/projects/*.
// Loaded after app.js (which owns "Scratch" single-snippet mode and the
// shared Monaco AMD loader / theme toggle); this file is additive and
// never touches app.js's editor or state.

(() => {
  const modeScratchBtn = document.getElementById('modeScratchBtn');
  const modeProjectsBtn = document.getElementById('modeProjectsBtn');
  const scratchView = document.getElementById('scratchView');
  const projectsView = document.getElementById('projectsView');

  const projectListEl = document.getElementById('projectList');
  const newProjectBtn = document.getElementById('newProjectBtn');
  const fileTreeEl = document.getElementById('fileTree');
  const fileTabsEl = document.getElementById('fileTabs');
  const saveStatusEl = document.getElementById('saveStatus');
  const runStatusPill = document.getElementById('runStatusPill');
  const runStartBtn = document.getElementById('runStartBtn');
  const runStopBtn = document.getElementById('runStopBtn');
  const runRestartBtn = document.getElementById('runRestartBtn');
  const previewLink = document.getElementById('previewLink');
  const projectEditorEmpty = document.getElementById('projectEditorEmpty');
  const projectEditorRoot = document.getElementById('projectEditor');
  const projectLogsEl = document.getElementById('projectLogs');
  const projectDiagnosticsEl = document.getElementById('projectDiagnostics');
  const projectPreviewFrame = document.getElementById('projectPreview');
  const logsPinBottom = document.getElementById('logsPinBottom');
  const pTabButtons = document.querySelectorAll('.ptab-btn');

  let monacoReady = false;
  let projectEditor = null;
  let projects = [];
  let currentProjectId = null;
  let currentTree = null;
  // path -> { model, dirty, saveTimer }
  const openFiles = new Map();
  let activeFilePath = null;
  let statusPollTimer = null;
  let logsSource = null;
  let diagDebounce = null;

  function languageForPath(path) {
    if (path.endsWith('.html')) return 'html';
    if (path.endsWith('.json')) return 'json';
    if (path.endsWith('.css')) return 'css';
    return 'spl';
  }

  async function api(path, opts) {
    const res = await fetch(path, Object.assign({ credentials: 'include' }, opts || {}));
    let body = null;
    try {
      body = await res.json();
    } catch (e) {
      body = null;
    }
    if (!res.ok) {
      const msg = (body && body.error) || `request failed (${res.status})`;
      throw new Error(msg);
    }
    return body;
  }

  function ensureMonaco() {
    if (window.monaco) {
      monacoReady = true;
      return Promise.resolve();
    }
    return new Promise((resolve) => {
      const iv = setInterval(() => {
        if (window.monaco) {
          clearInterval(iv);
          monacoReady = true;
          resolve();
        }
      }, 50);
    });
  }

  function ensureProjectEditor() {
    if (projectEditor) return;
    const isDark = document.documentElement.classList.contains('dark');
    projectEditor = monaco.editor.create(projectEditorRoot, {
      automaticLayout: true,
      fontSize: 14,
      fontFamily: 'JetBrains Mono, Fira Code, Menlo, monospace',
      minimap: { enabled: false },
      roundedSelection: true,
      scrollBeyondLastLine: false,
      padding: { top: 12, bottom: 12 },
      theme: isDark ? 'vs-dark' : 'vs',
      model: null,
    });
    projectEditor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
      if (activeFilePath) saveFile(activeFilePath);
    });

    // Project-aware completion/hover: contributes nothing (empty results)
    // unless Projects mode has an active project and file open, so it
    // coexists harmlessly with Scratch mode's static provider in app.js -
    // Monaco merges suggestions from every registered provider for a
    // language id.
    monaco.languages.registerCompletionItemProvider('spl', {
      triggerCharacters: ['.', '_', '"', '/'],
      provideCompletionItems: async (model, position) => {
        if (!currentProjectId || !activeFilePath || model !== openFiles.get(activeFilePath)?.model) {
          return { suggestions: [] };
        }
        const word = model.getWordUntilPosition(position);
        const range = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn,
        };
        try {
          const body = await api(`/api/projects/${currentProjectId}/tooling/completions`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              path: activeFilePath,
              content: model.getValue(),
              prefix: word.word,
              line: position.lineNumber,
              col: position.column,
            }),
          });
          const kindMap = { function: monaco.languages.CompletionItemKind.Function, keyword: monaco.languages.CompletionItemKind.Keyword, builtin: monaco.languages.CompletionItemKind.Function, variable: monaco.languages.CompletionItemKind.Variable, module: monaco.languages.CompletionItemKind.Module };
          const suggestions = (body.items || []).map((it) => ({
            label: it.label,
            kind: kindMap[it.kind] || monaco.languages.CompletionItemKind.Text,
            detail: it.detail || '',
            insertText: it.label,
            range,
          }));
          return { suggestions };
        } catch (e) {
          return { suggestions: [] };
        }
      },
    });

    monaco.languages.registerHoverProvider('spl', {
      provideHover: async (model, position) => {
        if (!currentProjectId || !activeFilePath || model !== openFiles.get(activeFilePath)?.model) {
          return null;
        }
        const word = model.getWordAtPosition(position);
        if (!word) return null;
        try {
          const body = await api(`/api/projects/${currentProjectId}/tooling/hover`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: activeFilePath, content: model.getValue(), line: position.lineNumber, col: position.column }),
          });
          if (!body.markdown) return null;
          return {
            range: new monaco.Range(position.lineNumber, word.startColumn, position.lineNumber, word.endColumn),
            contents: [{ value: body.markdown }],
          };
        } catch (e) {
          return null;
        }
      },
    });
  }

  // --- Mode switch ---

  function setMode(mode) {
    const showingProjects = mode === 'projects';
    scratchView.classList.toggle('hidden', showingProjects);
    projectsView.classList.toggle('hidden', !showingProjects);
    modeProjectsBtn.classList.toggle('is-active', showingProjects);
    modeScratchBtn.classList.toggle('is-active', !showingProjects);
    if (showingProjects) {
      ensureMonaco().then(() => {
        ensureProjectEditor();
        if (projects.length === 0) loadProjects();
      });
    }
  }

  modeScratchBtn.addEventListener('click', () => setMode('scratch'));
  modeProjectsBtn.addEventListener('click', () => setMode('projects'));

  // --- Projects list ---

  async function loadProjects() {
    try {
      const body = await api('/api/projects');
      projects = body.projects || [];
      renderProjectList();
    } catch (e) {
      projectListEl.innerHTML = `<div class="empty-note text-err">${escapeHtml(e.message)}</div>`;
    }
  }

  function renderProjectList() {
    projectListEl.innerHTML = '';
    for (const p of projects) {
      const row = document.createElement('div');
      row.className = `list-item list-row${p.id === currentProjectId ? ' is-active' : ''}`;
      const label = document.createElement('span');
      label.textContent = p.name;
      label.className = 'list-row__label';
      label.title = `${p.name} (${p.scaffold_kind})`;
      row.appendChild(label);
      const del = document.createElement('button');
      del.textContent = '×';
      del.title = 'Delete project';
      del.className = 'list-row__action';
      del.addEventListener('click', async (ev) => {
        ev.stopPropagation();
        if (!confirm(`Delete project "${p.name}"? This removes its files permanently.`)) return;
        try {
          await api(`/api/projects/${p.id}`, { method: 'DELETE' });
          if (currentProjectId === p.id) closeProject();
          await loadProjects();
        } catch (e) {
          alert(`Delete failed: ${e.message}`);
        }
      });
      row.appendChild(del);
      row.addEventListener('click', () => selectProject(p.id));
      projectListEl.appendChild(row);
    }
    if (projects.length === 0) {
      projectListEl.innerHTML = '<div class="empty-note">No projects yet.</div>';
    }
  }

  newProjectBtn.addEventListener('click', async () => {
    const name = prompt('Project name?');
    if (!name || !name.trim()) return;
    const kind = confirm('Use the full scaffold (SQLite + template views)?\nOK = app, Cancel = minimal (in-memory, no database)') ? 'app' : 'minimal';
    try {
      const body = await api('/api/projects', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim(), scaffold_kind: kind }),
      });
      await loadProjects();
      await selectProject(body.project.id);
    } catch (e) {
      alert(`Create failed: ${e.message}`);
    }
  });

  function closeProject() {
    currentProjectId = null;
    currentTree = null;
    for (const [, entry] of openFiles) entry.model.dispose();
    openFiles.clear();
    activeFilePath = null;
    fileTreeEl.innerHTML = '';
    fileTabsEl.innerHTML = '';
    projectEditor && projectEditor.setModel(null);
    projectEditorEmpty.classList.remove('hidden');
    stopStatusPolling();
    disconnectLogs();
    previewLink.classList.add('hidden');
    runStatusPill.textContent = 'idle';
    runStatusPill.className = 'status run-pill run-pill--idle';
  }

  async function selectProject(id) {
    if (currentProjectId === id) return;
    closeProject();
    currentProjectId = id;
    renderProjectList();
    await loadFileTree();
    startStatusPolling();
    connectLogs();
  }

  // --- File tree ---

  async function loadFileTree() {
    try {
      const body = await api(`/api/projects/${currentProjectId}/files`);
      currentTree = body.tree;
      renderFileTree();
    } catch (e) {
      fileTreeEl.innerHTML = `<div class="empty-note text-err">${escapeHtml(e.message)}</div>`;
    }
  }

  function renderFileTree() {
    fileTreeEl.innerHTML = '';
    if (!currentTree) return;
    fileTreeEl.appendChild(renderNode(currentTree, 0));
  }

  function renderNode(node, depth) {
    const wrap = document.createElement('div');
    if (node.type === 'dir') {
      if (depth > 0) {
        const label = document.createElement('div');
        label.textContent = node.name;
        label.className = 'file-tree__dir';
        label.style.setProperty('--depth', `${depth * 0.75}rem`);
        wrap.appendChild(label);
      }
      for (const child of node.children || []) {
        wrap.appendChild(renderNode(child, depth + 1));
      }
    } else {
      const row = document.createElement('button');
      row.type = 'button';
      row.textContent = node.name;
      row.className = `file-tree__file${activeFilePath === node.path ? ' is-active' : ''}`;
      row.style.setProperty('--depth', `${depth * 0.75}rem`);
      row.title = node.path;
      row.addEventListener('click', () => openFile(node.path));
      wrap.appendChild(row);
    }
    return wrap;
  }

  // --- File tabs / editing ---

  async function openFile(path) {
    if (!openFiles.has(path)) {
      try {
        const body = await api(`/api/projects/${currentProjectId}/files/${path}`);
        const model = monaco.editor.createModel(body.content, languageForPath(path));
        model.onDidChangeContent(() => {
          if (model === openFiles.get(path)?.model) markDirty(path);
        });
        openFiles.set(path, { model, dirty: false, saveTimer: null });
      } catch (e) {
        alert(`Open failed: ${e.message}`);
        return;
      }
    }
    activateFile(path);
  }

  function activateFile(path) {
    activeFilePath = path;
    const entry = openFiles.get(path);
    projectEditorEmpty.classList.add('hidden');
    projectEditor.setModel(entry.model);
    renderFileTabs();
    renderFileTree();
    scheduleDiagnostics(path);
  }

  function closeFile(path) {
    const entry = openFiles.get(path);
    if (!entry) return;
    if (entry.dirty) saveFile(path);
    entry.model.dispose();
    openFiles.delete(path);
    if (activeFilePath === path) {
      const remaining = Array.from(openFiles.keys());
      if (remaining.length > 0) {
        activateFile(remaining[remaining.length - 1]);
      } else {
        activeFilePath = null;
        projectEditor.setModel(null);
        projectEditorEmpty.classList.remove('hidden');
        renderFileTabs();
      }
    } else {
      renderFileTabs();
    }
  }

  function renderFileTabs() {
    fileTabsEl.innerHTML = '';
    for (const [path, entry] of openFiles) {
      const tab = document.createElement('div');
      tab.className = `file-tab${path === activeFilePath ? ' is-active' : ''}`;
      const name = document.createElement('span');
      name.textContent = path.split('/').pop();
      name.title = path;
      tab.appendChild(name);
      if (entry.dirty) {
        const dirty = document.createElement('span');
        dirty.className = 'file-tab__dirty';
        dirty.textContent = '●';
        tab.appendChild(dirty);
      }
      const close = document.createElement('button');
      close.type = 'button';
      close.textContent = '×';
      close.className = 'file-tab__close';
      close.addEventListener('click', (ev) => {
        ev.stopPropagation();
        closeFile(path);
      });
      tab.appendChild(close);
      tab.addEventListener('click', () => activateFile(path));
      fileTabsEl.appendChild(tab);
    }
  }

  function markDirty(path) {
    const entry = openFiles.get(path);
    if (!entry) return;
    entry.dirty = true;
    saveStatusEl.textContent = 'unsaved changes';
    renderFileTabs();
    if (entry.saveTimer) clearTimeout(entry.saveTimer);
    entry.saveTimer = setTimeout(() => saveFile(path), 1000);
    scheduleDiagnostics(path);
  }

  async function saveFile(path) {
    const entry = openFiles.get(path);
    if (!entry) return;
    if (entry.saveTimer) {
      clearTimeout(entry.saveTimer);
      entry.saveTimer = null;
    }
    try {
      await api(`/api/projects/${currentProjectId}/files/${path}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content: entry.model.getValue() }),
      });
      entry.dirty = false;
      saveStatusEl.textContent = 'saved';
      renderFileTabs();
    } catch (e) {
      saveStatusEl.textContent = `save failed: ${e.message}`;
    }
  }

  function scheduleDiagnostics(path) {
    if (diagDebounce) clearTimeout(diagDebounce);
    diagDebounce = setTimeout(() => refreshDiagnostics(path), 400);
  }

  async function refreshDiagnostics(path) {
    const entry = openFiles.get(path);
    if (!entry || !currentProjectId) return;
    try {
      const body = await api(`/api/projects/${currentProjectId}/tooling/diagnostics`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path, content: entry.model.getValue() }),
      });
      const diags = body.diagnostics || [];
      const severityMap = { error: monaco.MarkerSeverity.Error, warning: monaco.MarkerSeverity.Warning, info: monaco.MarkerSeverity.Info };
      const markers = diags.map((d) => ({
        severity: severityMap[d.severity] || monaco.MarkerSeverity.Error,
        message: d.message,
        startLineNumber: d.line || 1,
        startColumn: d.column || 1,
        endLineNumber: d.line || 1,
        endColumn: (d.column || 1) + 1,
      }));
      monaco.editor.setModelMarkers(entry.model, 'spl-ide', markers);
      projectDiagnosticsEl.textContent = diags.length
        ? diags.map((d) => `${d.severity} ${path}:${d.line || 0}:${d.column || 0} ${d.message}`).join('\n\n')
        : 'No diagnostics.';
    } catch (e) {
      // Diagnostics are best-effort; swallow transient failures.
    }
  }

  // --- Run lifecycle ---

  function statusLabel(state) {
    return state || 'idle';
  }

  function applyStatus(status) {
    const state = statusLabel(status.state);
    runStatusPill.textContent = state;
    runStatusPill.className = `status run-pill run-pill--${state}`;
    if (status.state === 'running' && status.port) {
      const url = `${location.protocol}//${location.hostname}:${status.port}/`;
      previewLink.href = url;
      previewLink.classList.remove('hidden');
      projectPreviewFrame.src = url;
    } else {
      previewLink.classList.add('hidden');
      projectPreviewFrame.removeAttribute('src');
    }
  }

  async function refreshStatus() {
    if (!currentProjectId) return;
    try {
      const status = await api(`/api/projects/${currentProjectId}/run/status`);
      applyStatus(status);
    } catch (e) {
      // ignore transient polling failures
    }
  }

  function startStatusPolling() {
    stopStatusPolling();
    refreshStatus();
    statusPollTimer = setInterval(refreshStatus, 2000);
  }

  function stopStatusPolling() {
    if (statusPollTimer) {
      clearInterval(statusPollTimer);
      statusPollTimer = null;
    }
  }

  runStartBtn.addEventListener('click', async () => {
    if (!currentProjectId) return;
    try {
      applyStatus(await api(`/api/projects/${currentProjectId}/run/start`, { method: 'POST' }));
    } catch (e) {
      alert(`Start failed: ${e.message}`);
    }
  });
  runStopBtn.addEventListener('click', async () => {
    if (!currentProjectId) return;
    try {
      applyStatus(await api(`/api/projects/${currentProjectId}/run/stop`, { method: 'POST' }));
    } catch (e) {
      alert(`Stop failed: ${e.message}`);
    }
  });
  runRestartBtn.addEventListener('click', async () => {
    if (!currentProjectId) return;
    try {
      applyStatus(await api(`/api/projects/${currentProjectId}/run/restart`, { method: 'POST' }));
    } catch (e) {
      alert(`Restart failed: ${e.message}`);
    }
  });

  // --- Logs (SSE) ---

  function connectLogs() {
    disconnectLogs();
    projectLogsEl.textContent = '';
    logsSource = new EventSource(`/api/projects/${currentProjectId}/logs/stream`);
    logsSource.addEventListener('log', (ev) => {
      try {
        const line = JSON.parse(ev.data);
        appendLogLine(line);
      } catch (e) {
        // ignore malformed frames
      }
    });
  }

  function disconnectLogs() {
    if (logsSource) {
      logsSource.close();
      logsSource = null;
    }
  }

  function appendLogLine(line) {
    const prefix = line.stream === 'stderr' ? '[stderr] ' : '';
    projectLogsEl.textContent += `${prefix}${line.line}\n`;
    if (logsPinBottom.checked) {
      projectLogsEl.scrollTop = projectLogsEl.scrollHeight;
    }
  }

  // --- Bottom panel tabs ---

  for (const btn of pTabButtons) {
    btn.addEventListener('click', () => {
      for (const b of pTabButtons) b.classList.remove('is-active');
      btn.classList.add('is-active');
      for (const panel of document.querySelectorAll('.ppanel')) {
        panel.classList.toggle('hidden', panel.dataset.ppanel !== btn.dataset.ptab);
      }
    });
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }
})();
