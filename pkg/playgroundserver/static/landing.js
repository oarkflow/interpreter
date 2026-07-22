(() => {
  const landing = document.getElementById('landingPage');
  const playground = document.getElementById('playgroundApp');
  const back = document.getElementById('playgroundBack');
  const menu = document.getElementById('landingMenu');
  const nav = document.querySelector('.site-nav');

  const demoExamples = {
    rules: {
      file: 'routing.spl',
      source: [
        'let order = {"id": "ORD-1042", "total": 7200};',
        'let queue = if (order.total > 5000) { "review" } else { "standard" };',
        '{"id": order.id, "queue": queue};',
      ].join('\n'),
      display: [
        '<span class="ln">1</span><span class="kw">let</span> order = { <span class="str">"id"</span>: <span class="str">"ORD-1042"</span>, <span class="str">"total"</span>: <span class="num">7200</span> };',
        '<span class="ln">2</span><span class="kw">let</span> queue = <span class="kw">if</span> (order.total &gt; <span class="num">5000</span>) {',
        '<span class="ln">3</span>  <span class="str">"review"</span>',
        '<span class="ln">4</span>} <span class="kw">else</span> {',
        '<span class="ln">5</span>  <span class="str">"standard"</span>',
        '<span class="ln">6</span>};',
        '<span class="ln">7</span>{ <span class="str">"id"</span>: order.id, <span class="str">"queue"</span>: queue };',
      ].join('\n'),
      fallback: '{id: ORD-1042, queue: review}',
    },
    data: {
      file: 'orders.spl',
      source: [
        'let orders = [{"id": "A-17", "total": 680}, {"id": "B-42", "total": 1400}, {"id": "C-08", "total": 2200}];',
        'let large = orders.filter(order => order.total >= 1000);',
        'large.map(order => order.id);',
      ].join('\n'),
      display: [
        '<span class="ln">1</span><span class="kw">let</span> orders = [',
        '<span class="ln">2</span>  { id: <span class="str">"A-17"</span>, total: <span class="num">680</span> },',
        '<span class="ln">3</span>  { id: <span class="str">"B-42"</span>, total: <span class="num">1400</span> },',
        '<span class="ln">4</span>  { id: <span class="str">"C-08"</span>, total: <span class="num">2200</span> }',
        '<span class="ln">5</span>];',
        '<span class="ln">6</span><span class="kw">let</span> large = orders.<span class="fn">filter</span>(o =&gt; o.total &gt;= <span class="num">1000</span>);',
        '<span class="ln">7</span>large.<span class="fn">map</span>(o =&gt; o.id);',
      ].join('\n'),
      fallback: '[B-42, C-08]',
    },
    automation: {
      file: 'daily_report.spl',
      source: [
        'let job = {"name": "daily-report", "schedule": "0 8 * * *", "enabled": true};',
        'sprintf("%s scheduled for %s", job.name, job.schedule);',
      ].join('\n'),
      display: [
        '<span class="ln">1</span><span class="cm">// A job definition can be scheduled by the host</span>',
        '<span class="ln">2</span><span class="kw">let</span> job = {',
        '<span class="ln">3</span>  name: <span class="str">"daily-report"</span>,',
        '<span class="ln">4</span>  schedule: <span class="str">"0 8 * * *"</span>,',
        '<span class="ln">5</span>  enabled: <span class="kw">true</span>',
        '<span class="ln">6</span>};',
        '<span class="ln">7</span><span class="fn">sprintf</span>(<span class="str">"%s scheduled for %s"</span>,',
        '<span class="ln">8</span>  job.name, job.schedule);',
      ].join('\n'),
      fallback: 'daily-report scheduled for 0 8 * * *',
    },
  };

  const profiles = {
    untrusted: {
      description: 'Strict host protection with worker-process execution and read-only filesystem access rooted at the script directory.',
      isolation: 'isolated worker',
      capabilities: { filesystem: ['LIMITED', 'script directory · read'], network: ['DENIED', '—'], database: ['DENIED', '—'], process: ['DENIED', '—'], server: ['DENIED', '—'] },
    },
    readonly: {
      description: 'A named read-only preset with strict limits and the same narrow filesystem boundary as the untrusted baseline.',
      isolation: 'isolated worker',
      capabilities: { filesystem: ['LIMITED', 'module directory · read'], network: ['DENIED', '—'], database: ['DENIED', '—'], process: ['DENIED', '—'], server: ['DENIED', '—'] },
    },
    automation: {
      description: 'Adds controlled execution, filesystem writes, and system operations to the protected baseline for automation workloads.',
      isolation: 'isolated worker',
      capabilities: { filesystem: ['ALLOWED', 'scoped read · write'], network: ['DENIED', '—'], database: ['DENIED', '—'], process: ['ALLOWED', 'exec · system'], server: ['DENIED', '—'] },
    },
    server: {
      description: 'Adds network and server capabilities to the protected baseline for scripts that host HTTP workloads.',
      isolation: 'isolated worker',
      capabilities: { filesystem: ['LIMITED', 'module directory · read'], network: ['ALLOWED', 'network policy'], database: ['DENIED', '—'], process: ['DENIED', '—'], server: ['ALLOWED', 'listen · routes'] },
    },
  };

  const quickstarts = {
    install: { label: 'terminal', code: '<code><i>$</i> go -C cmd/interpreter build -o interpreter .</code>', copy: 'go -C cmd/interpreter build -o interpreter .', step: 'Build the full CLI', note: 'The full cmd/interpreter build includes the complete optional plugin surface.' },
    execute: { label: 'main.go', code: '<code><b>import</b> <q>"github.com/oarkflow/interpreter"</q>\n\n<em>result</em>, <em>err</em> := interpreter.Exec(\n    <q>"let x = 40; x + 2;"</q>,\n    nil,\n)</code>', copy: 'result, err := interpreter.Exec("let x = 40; x + 2;", nil)', step: 'Execute SPL from Go', note: 'Pass a map as the second argument to inject application data into script scope.' },
    serve: { label: 'terminal', code: '<code><i>$</i> ./interpreter --playground\n\n<span class="cm"># open http://localhost:8080</span></code>', copy: './interpreter --playground', step: 'Launch the browser workspace', note: 'Use Scratch for examples or Projects for multi-file SPL applications.' },
  };

  function setSelected(buttons, active) {
    buttons.forEach((button) => {
      const selected = button === active;
      button.classList.toggle('is-active', selected);
      button.setAttribute('aria-selected', String(selected));
    });
  }

  function showPlayground(updateHash = true) {
    landing.classList.add('hidden');
    playground.classList.remove('hidden');
    document.body.classList.add('is-playground');
    if (updateHash) history.pushState({ view: 'playground' }, '', '#playground');
    window.dispatchEvent(new Event('resize'));
  }

  function showLanding(updateHash = true) {
    playground.classList.add('hidden');
    landing.classList.remove('hidden');
    document.body.classList.remove('is-playground');
    if (updateHash) history.pushState({ view: 'landing' }, '', location.pathname + location.search);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  document.querySelectorAll('[data-open-playground]').forEach((button) => {
    button.addEventListener('click', () => showPlayground());
  });

  back.addEventListener('click', () => showLanding());
  window.addEventListener('popstate', () => location.hash === '#playground' ? showPlayground(false) : showLanding(false));

  menu.addEventListener('click', () => {
    const open = nav.classList.toggle('is-open');
    menu.setAttribute('aria-expanded', String(open));
  });
  document.querySelectorAll('.site-nav__links a').forEach((link) => link.addEventListener('click', () => {
    nav.classList.remove('is-open');
    menu.setAttribute('aria-expanded', 'false');
  }));

  async function copyText(value) {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(value);
      return;
    }
    const input = document.createElement('textarea');
    input.value = value;
    input.setAttribute('readonly', '');
    input.style.position = 'fixed';
    input.style.opacity = '0';
    document.body.appendChild(input);
    input.select();
    document.execCommand('copy');
    input.remove();
  }

  document.querySelectorAll('[data-copy-command]').forEach((button) => {
    button.addEventListener('click', async () => {
      const original = button.textContent;
      try {
        await copyText(button.dataset.copyCommand);
        button.textContent = 'Copied ✓';
      } catch (_) {
        button.textContent = 'Copy failed';
      }
      setTimeout(() => { button.textContent = original; }, 1600);
    });
  });

  const demoButtons = Array.from(document.querySelectorAll('[data-demo]'));
  const demoCode = document.getElementById('demoCode');
  const demoFilename = document.getElementById('demoFilename');
  const demoResult = document.getElementById('demoResult');
  const demoStatus = document.getElementById('demoStatus');
  const demoRun = document.getElementById('demoRun');
  let activeDemo = 'rules';

  function selectDemo(name) {
    const example = demoExamples[name];
    if (!example) return;
    activeDemo = name;
    setSelected(demoButtons, demoButtons.find((button) => button.dataset.demo === name));
    demoCode.classList.add('is-switching');
    window.setTimeout(() => {
      demoFilename.textContent = example.file;
      demoCode.innerHTML = example.display;
      demoResult.textContent = example.fallback;
      demoStatus.textContent = 'READY';
      demoStatus.className = '';
      demoCode.classList.remove('is-switching');
    }, 100);
  }

  demoButtons.forEach((button) => button.addEventListener('click', () => selectDemo(button.dataset.demo)));
  selectDemo(activeDemo);

  demoRun.addEventListener('click', async () => {
    const example = demoExamples[activeDemo];
    demoRun.disabled = true;
    demoRun.classList.add('is-running');
    demoRun.firstChild.textContent = 'Running ';
    demoStatus.textContent = 'RUNNING';
    demoStatus.className = 'is-running';
    demoResult.textContent = 'Evaluating with the SPL runtime…';
    try {
      const response = await fetch('/api/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ code: example.source, render_mode: 'off' }),
      });
      const payload = await response.json();
      if (!response.ok || payload.error) throw new Error(payload.error || 'Execution unavailable');
      demoResult.textContent = payload.result || payload.output || example.fallback;
      demoStatus.textContent = 'SUCCESS';
      demoStatus.className = 'is-success';
    } catch (error) {
      demoResult.textContent = String(error.message || error);
      demoStatus.textContent = 'NOT RUN';
      demoStatus.className = '';
    } finally {
      demoRun.disabled = false;
      demoRun.classList.remove('is-running');
      demoRun.firstChild.textContent = 'Run ';
    }
  });

  const productButtons = Array.from(document.querySelectorAll('[data-product-tab]'));
  const productPanels = Array.from(document.querySelectorAll('[data-product-panel]'));
  productButtons.forEach((button) => button.addEventListener('click', () => {
    setSelected(productButtons, button);
    productPanels.forEach((panel) => panel.classList.toggle('is-active', panel.dataset.productPanel === button.dataset.productTab));
  }));

  const profileButtons = Array.from(document.querySelectorAll('[data-profile]'));
  profileButtons.forEach((button) => button.addEventListener('click', () => {
    const profile = profiles[button.dataset.profile];
    setSelected(profileButtons, button);
    document.getElementById('profileName').textContent = button.dataset.profile;
    document.getElementById('profileDescription').textContent = profile.description;
    document.getElementById('isolationText').textContent = profile.isolation;
    Object.entries(profile.capabilities).forEach(([name, values]) => {
      const row = document.querySelector('[data-capability="' + name + '"]');
      const badge = row.querySelector('.access');
      const access = values[0];
      badge.textContent = access;
      badge.className = 'access access--' + (access === 'ALLOWED' ? 'allowed' : access === 'LIMITED' ? 'limited' : 'denied');
      row.querySelector('code').textContent = values[1];
    });
  }));

  const codeButtons = Array.from(document.querySelectorAll('[data-code-tab]'));
  const quickstartCode = document.getElementById('quickstartCode');
  const quickstartLabel = document.getElementById('quickstartLabel');
  const quickstartStep = document.getElementById('quickstartStep');
  const quickstartNote = document.getElementById('quickstartNote');
  const quickstartCopy = document.getElementById('quickstartCopy');
  codeButtons.forEach((button) => button.addEventListener('click', () => {
    const item = quickstarts[button.dataset.codeTab];
    setSelected(codeButtons, button);
    quickstartLabel.textContent = item.label;
    quickstartCode.innerHTML = item.code;
    quickstartStep.textContent = item.step;
    quickstartNote.textContent = item.note;
    quickstartCopy.dataset.copyCommand = item.copy;
  }));

  if ('IntersectionObserver' in window) {
    const navLinks = Array.from(document.querySelectorAll('.site-nav__links a'));
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return;
        navLinks.forEach((link) => link.classList.toggle('is-current', link.getAttribute('href') === '#' + entry.target.id));
      });
    }, { rootMargin: '-25% 0px -65%' });
    document.querySelectorAll('#product, #security, #developers').forEach((section) => observer.observe(section));
  }

  if (location.hash === '#playground') showPlayground(false);
})();
