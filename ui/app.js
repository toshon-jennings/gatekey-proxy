/* ──────────────────────────────────────────────────────────────
   AI Proxy — dashboard behaviour

   The signal path mirrors server/proxy.go:routeModel() exactly.
   If that function changes, change ROUTES + resolve() with it,
   or the panel starts lying about what the proxy does.
   ────────────────────────────────────────────────────────────── */

const ENDPOINT = 'http://127.0.0.1:8181/v1';

/* Destinations you can compose a request for. `prefix` is what the
   proxy matches on; OpenAI is the fall-through, so it takes none. */
const DESTINATIONS = {
  groq:       { prefix: 'groq/',       host: 'api.groq.com',  path: '/openai/v1/chat/completions' },
  openrouter: { prefix: 'openrouter/', host: 'openrouter.ai', path: '/api/v1/chat/completions' },
  openai:     { prefix: '',            host: 'api.openai.com', path: '/v1/chat/completions' },
  opencode:   { prefix: 'opencode/',   host: 'opencode.ai',   path: '/zen/v1/chat/completions' },
  deepinfra:  { prefix: 'deepinfra/',  host: 'api.deepinfra.com', path: '/v1/openai/chat/completions' },
  together:   { prefix: 'together/',   host: 'api.together.xyz', path: '/v1/chat/completions' },
};

const ROUTABLE = Object.keys(DESTINATIONS);

/* Model ids come from the input and provider names come from
   config.json, so neither is safe to drop into markup as-is. */
const esc = (s) => String(s).replace(/[&<>"']/g, (c) => (
  { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
));

/* Mirror of routeModel(). Returns what the upstream actually receives. */
function resolve(sentModel) {
  for (const name of Object.keys(DESTINATIONS)) {
    const { prefix, host, path } = DESTINATIONS[name];
    if (prefix && sentModel.startsWith(prefix)) {
      return { provider: name, model: sentModel.slice(prefix.length), host, path, ok: true };
    }
  }
  // Everything else falls through to OpenAI — and the prefix is NOT stripped.
  const { host, path } = DESTINATIONS.openai;
  const stray = sentModel.includes('/') ? sentModel.split('/')[0] : null;
  return { provider: 'openai', model: sentModel, host, path, ok: !stray, stray };
}

document.addEventListener('DOMContentLoaded', () => {
  const $ = (id) => document.getElementById(id);

  const dest      = $('dest');
  const model     = $('model');
  const pathBody  = $('path-body');
  const verdict   = $('verdict');
  const sentModel = $('sent-model');
  const ops       = $('ops');
  const paneOut   = $('pane-out');
  const upHost    = $('up-host');
  const upModel   = $('up-model');
  const upAuth    = $('up-auth');
  const upNote    = $('up-note');
  const toast     = $('toast');

  let keysOnFile = [];
  let toastTimer;

  /* ── Feedback ─────────────────────────────────────────────── */

  function say(message) {
    clearTimeout(toastTimer);
    toast.textContent = message;
    toast.classList.remove('hidden');
    toastTimer = setTimeout(() => toast.classList.add('hidden'), 2200);
  }

  async function copy(text, message) {
    try {
      await navigator.clipboard.writeText(text);
      say(message);
    } catch {
      say('Copy blocked by the browser — select the text instead.');
    }
  }

  /* ── Signal path ──────────────────────────────────────────── */

  function currentSentModel() {
    const id = model.value.trim() || 'gpt-4o';
    return DESTINATIONS[dest.value].prefix + id;
  }

  function render({ animate = true } = {}) {
    const sent = currentSentModel();
    const up = resolve(sent);
    const hasKey = keysOnFile.includes(up.provider);
    const faulted = !up.ok || !hasKey;

    sentModel.textContent = sent;

    // What the proxy does, in order.
    const steps = [];
    if (up.ok && DESTINATIONS[up.provider].prefix) {
      steps.push({ html: `Strip <code>${esc(DESTINATIONS[up.provider].prefix)}</code> from the model id` });
    } else if (up.ok) {
      steps.push({ html: 'No prefix, so the request takes the default route' });
    } else {
      const prefixes = Object.keys(DESTINATIONS)
        .filter((n) => DESTINATIONS[n].prefix)
        .map((n) => `<code>${esc(DESTINATIONS[n].prefix)}</code>`)
        .join(', ');
      steps.push({ fault: true, html: `No rule for <code>${esc(up.stray)}/</code> — only ${prefixes} are matched` });
      steps.push({ fault: true, html: 'Falls through to OpenAI with the prefix left on' });
    }

    steps.push(hasKey
      ? { html: `Swap <code>sk-dummy</code> for your <code>${esc(up.provider)}</code> key` }
      : { fault: true, html: `No <code>${esc(up.provider)}</code> key on file` });

    steps.push({ html: `Forward to <code>${esc(up.host)}</code>` });

    ops.innerHTML = steps
      .map((s) => `<li${s.fault ? ' class="is-fault"' : ''}>${s.html}</li>`)
      .join('');

    // What actually arrives.
    upHost.textContent = up.host;
    upModel.textContent = up.model;
    upModel.classList.toggle('is-fault', !up.ok);
    upAuth.textContent = hasKey ? `Bearer <${up.provider} key>` : `Bearer <no ${up.provider} key>`;
    upAuth.classList.toggle('is-fault', !hasKey);
    paneOut.classList.toggle('is-fault', faulted);

    if (!up.ok) {
      upNote.textContent = `OpenAI will reject "${up.model}" — it is not a model id OpenAI knows. Pick OpenRouter as the destination if you want to reach ${up.stray} models.`;
      upNote.className = 'pane__note is-fault';
    } else if (!hasKey) {
      upNote.textContent = `The proxy will answer 401 until a ${up.provider} key is on file. Add one under Keys.`;
      upNote.className = 'pane__note is-fault';
    } else {
      upNote.textContent = `${up.host}${up.path}`;
      upNote.className = 'pane__note';
    }

    // Verdict.
    if (!up.ok) {
      verdict.textContent = `Unroutable — ${up.stray}/ has no rule`;
    } else if (!hasKey) {
      verdict.textContent = `Blocked — no ${up.provider} key`;
    } else {
      verdict.textContent = `Routes to ${up.host}`;
    }
    verdict.classList.toggle('is-fault', faulted);

    pathBody.classList.toggle('is-fault', faulted);

    // Presets reflect the current route.
    document.querySelectorAll('#presets button').forEach((b) => {
      b.setAttribute('aria-pressed', String(b.dataset.dest === dest.value && b.dataset.model === model.value.trim()));
    });

    if (animate) {
      pathBody.classList.remove('is-live');
      void pathBody.offsetWidth; // restart the pulse
      pathBody.classList.add('is-live');
    }

    renderHandoff(sent);
  }

  dest.addEventListener('change', () => render());
  model.addEventListener('input', () => render());

  document.querySelectorAll('#presets button').forEach((b) => {
    b.addEventListener('click', () => {
      dest.value = b.dataset.dest;
      model.value = b.dataset.model;
      render();
    });
  });

  /* ── Keys ─────────────────────────────────────────────────── */

  const keys = $('keys');
  const dialog = $('key-dialog');
  const keyForm = $('key-form');
  const kProvider = $('k-provider');
  const kSecret = $('k-secret');
  const kError = $('k-error');

  async function loadKeys() {
    try {
      const res = await fetch('/api/keys');
      keysOnFile = (await res.json()) || [];
    } catch {
      keysOnFile = [];
      say('Could not read keys — is the proxy still running?');
    }
    renderKeys();
    render({ animate: false });
  }

  function renderKeys() {
    const all = [...new Set([...ROUTABLE, ...keysOnFile])];

    keys.innerHTML = all.map((p) => {
      const onFile = keysOnFile.includes(p);
      const routable = ROUTABLE.includes(p);
      const name = esc(p);
      return `
        <li class="key ${onFile ? '' : 'key--empty'}">
          <span class="key__dot" aria-hidden="true"></span>
          <span class="key__name">${name}</span>
          ${routable ? '' : '<span class="key__note">no route uses this</span>'}
          <span class="key__state">${onFile ? 'on file' : 'not set'}</span>
          <button type="button" class="key__act" data-add="${name}">${onFile ? 'Replace' : 'Add'}</button>
          ${onFile ? `<button type="button" class="key__act key__act--remove" data-remove="${name}">Remove</button>` : ''}
        </li>`;
    }).join('');
  }

  keys.addEventListener('click', (e) => {
    const add = e.target.closest('[data-add]');
    if (add) return openSheet(add.dataset.add);

    const remove = e.target.closest('[data-remove]');
    if (remove) return removeKey(remove.dataset.remove);
  });

  function openSheet(provider = '') {
    kError.classList.add('hidden');
    keyForm.reset();
    kProvider.value = provider;
    dialog.showModal();
    (provider ? kSecret : kProvider).focus();
  }

  $('add-key').addEventListener('click', () => openSheet());
  $('k-cancel').addEventListener('click', () => dialog.close());

  keyForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const provider = kProvider.value.trim().toLowerCase();
    const key = kSecret.value.trim();
    if (!provider || !key) return;

    try {
      const res = await fetch('/api/keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider, key }),
      });
      if (!res.ok) throw new Error(await res.text());

      dialog.close();
      say(`Saved the ${provider} key`);
      await loadKeys();
    } catch (err) {
      kError.textContent = `Could not save the key: ${err.message || 'the proxy did not respond'}`;
      kError.classList.remove('hidden');
    }
  });

  async function removeKey(provider) {
    if (!confirm(`Remove the ${provider} key from ~/.config/ai-proxy/config.json?`)) return;
    try {
      const res = await fetch(`/api/keys?provider=${encodeURIComponent(provider)}`, { method: 'DELETE' });
      if (!res.ok) throw new Error();
      say(`Removed the ${provider} key`);
      await loadKeys();
    } catch {
      say(`Could not remove the ${provider} key`);
    }
  }

  /* ── Handoff ──────────────────────────────────────────────── */

  const panels = {
    'tab-shell': $('p-shell'),
    'tab-agent': $('p-agent'),
    'tab-python': $('p-python'),
  };
  const tabButtons = [...document.querySelectorAll('.tabs button')];

  function renderHandoff(sent) {
    panels['tab-shell'].textContent =
      `export OPENAI_BASE_URL="${ENDPOINT}"\n` +
      `export OPENAI_API_KEY="sk-dummy"\n` +
      `export OPENAI_MODEL="${sent}"`;

    panels['tab-agent'].textContent =
      `Use my local proxy for the next tasks. Set the OpenAI-compatible base URL ` +
      `to ${ENDPOINT}, the API key to sk-dummy, and the model to ${sent}. ` +
      `The proxy adds the real provider key, so leave the dummy key as it is.`;

    panels['tab-python'].textContent =
      `from openai import OpenAI\n\n` +
      `client = OpenAI(base_url="${ENDPOINT}", api_key="sk-dummy")\n\n` +
      `response = client.chat.completions.create(\n` +
      `    model="${sent}",\n` +
      `    messages=[{"role": "user", "content": "Hello"}],\n` +
      `)\n` +
      `print(response.choices[0].message.content)`;
  }

  function selectTab(id) {
    tabButtons.forEach((b) => {
      const on = b.id === id;
      b.setAttribute('aria-selected', String(on));
      b.tabIndex = on ? 0 : -1;
      panels[b.id].classList.toggle('hidden', !on);
    });
  }

  tabButtons.forEach((b, i) => {
    b.addEventListener('click', () => selectTab(b.id));
    b.addEventListener('keydown', (e) => {
      const step = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
      if (!step) return;
      e.preventDefault();
      const next = tabButtons[(i + step + tabButtons.length) % tabButtons.length];
      selectTab(next.id);
      next.focus();
    });
  });

  $('copy-handoff').addEventListener('click', () => {
    const open = tabButtons.find((b) => b.getAttribute('aria-selected') === 'true');
    copy(panels[open.id].textContent, 'Copied');
  });

  /* ── Bench test ───────────────────────────────────────────── */

  const benchForm = $('bench');
  const prompt = $('prompt');
  const send = $('send');
  const reply = $('reply');
  const latency = $('latency');
  const rawWrap = $('raw-wrap');
  const raw = $('raw');

  benchForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const text = prompt.value.trim();
    if (!text) return;

    const sent = currentSentModel();

    send.disabled = true;
    latency.textContent = '';
    rawWrap.classList.add('hidden');
    reply.classList.remove('is-fault');
    reply.innerHTML = '<span class="reply__wait">Sending…</span>';

    const started = performance.now();
    try {
      const res = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model: sent, messages: [{ role: 'user', content: text }] }),
      });

      latency.textContent = `${Math.round(performance.now() - started)} ms`;

      // Errors from the proxy come back as plain text, not JSON.
      const body = await res.text();
      let data = null;
      try { data = JSON.parse(body); } catch { /* keep the text */ }

      raw.textContent = data ? JSON.stringify(data, null, 2) : body;
      rawWrap.classList.remove('hidden');

      const answer = data?.choices?.[0]?.message?.content;
      if (answer) {
        reply.textContent = answer;
      } else {
        reply.textContent = data?.error?.message || body.trim() || `The upstream answered ${res.status} with no body.`;
        reply.classList.add('is-fault');
      }

      pathBody.classList.remove('is-live');
      void pathBody.offsetWidth;
      pathBody.classList.add('is-live');
    } catch (err) {
      latency.textContent = '';
      reply.textContent = `Could not reach the proxy at ${ENDPOINT}. Check that "ai-proxy start" is still running.`;
      reply.classList.add('is-fault');
    } finally {
      send.disabled = false;
    }
  });

  /* ── Copy the endpoint ────────────────────────────────────── */

  document.querySelectorAll('[data-copy]').forEach((el) => {
    el.addEventListener('click', () => copy(el.dataset.copy, 'Copied'));
  });

  /* ── Go ───────────────────────────────────────────────────── */

  render({ animate: false });
  loadKeys();
});
