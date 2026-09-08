// HTMX progressive enhancement: ordinary form posts work without JavaScript.
document.addEventListener('htmx:responseError',function(event){const box=document.getElementById('global-error');if(box){box.hidden=false;box.textContent='That request could not finish. Your draft is still here. Please try again.';box.scrollIntoView({block:'nearest'});}});
document.addEventListener('htmx:sendError',function(){const box=document.getElementById('global-error');if(box){box.hidden=false;box.textContent='Connection interrupted. Your draft is still here. Try again when you are back online.';}});

// Drafts stay in this tab and are scoped to the author and conversation. Never
// store access keys, CSRF tokens, or other fields from account/settings forms.
(() => {
  const prefix = 'ajent-draft:';
  const fields = ['kind', 'audience', 'title', 'body', 'request_key'];
  const states = [];
  for (const form of document.querySelectorAll('form[data-draft-key]')) {
    const key = prefix + form.dataset.draftKey;
    const feedback = document.createElement('div');
    feedback.className = 'draft-feedback';
    feedback.hidden = true;
    const status = document.createElement('p');
    status.className = 'fine';
    status.setAttribute('role', 'status');
    const discard = document.createElement('button');
    discard.type = 'button';
    discard.className = 'text-button';
    discard.textContent = 'Discard draft';
    feedback.append(status, discard);
    form.append(feedback);
    const state = {form, completed: false, unsaved: false};
    states.push(state);
    const values = () => Object.fromEntries(fields.flatMap(name => {
      const field = form.elements.namedItem(name);
      return field ? [[name, field.value]] : [];
    }));
    const hasContent = data => Boolean(data.title?.trim() || data.body?.trim());
    const message = text => { if (status.textContent !== text) status.textContent = text; };
    const remove = () => { try { sessionStorage.removeItem(key); } catch {} };
    state.save = () => {
      if (state.completed) return;
      const data = values();
      if (!hasContent(data)) {
        remove(); state.unsaved = false; feedback.hidden = true; return;
      }
      feedback.hidden = false;
      try {
        sessionStorage.setItem(key, JSON.stringify({version: 1, data}));
        state.unsaved = false;
        message('Draft saved in this tab.');
      } catch {
        state.unsaved = true;
        message('Draft could not be saved. Keep this page open until you publish or copy it.');
      }
    };
    state.clear = () => { state.completed = true; state.unsaved = false; remove(); feedback.hidden = true; };
    let missingAudienceOption;
    try {
      const saved = JSON.parse(sessionStorage.getItem(key));
      if (saved?.version === 1 && saved.data && hasContent(saved.data)) {
        let audienceChanged = false;
        for (const name of fields) {
          const field = form.elements.namedItem(name), value = saved.data[name];
          if (!field || typeof value !== 'string') continue;
          if (field instanceof HTMLSelectElement && ![...field.options].some(option => option.value === value)) {
            if (name === 'audience') {
              missingAudienceOption = new Option('Choose where to share', '', true, true);
              field.add(missingAudienceOption, 0);
              field.required = true; audienceChanged = true;
            }
            continue;
          }
          if (field.maxLength > 0 && value.length > field.maxLength) continue;
          if (name === 'request_key' && !/^[0-9a-f-]{36}$/i.test(value)) continue;
          field.value = value;
        }
        const composer = form.closest('details');
        if (composer) composer.open = true;
        feedback.hidden = false;
        message(audienceChanged ? 'Draft restored. Choose where to share it before publishing.' : 'Draft restored from this tab.');
      }
    } catch { remove(); }
    form.addEventListener('input', state.save);
    form.addEventListener('change', state.save);
    discard.addEventListener('click', () => {
      if (!window.confirm('Discard this unfinished post?')) return;
      missingAudienceOption?.remove();
      missingAudienceOption = null;
      form.reset();
      const requestKey = form.elements.namedItem('request_key');
      if (requestKey) requestKey.value = crypto.randomUUID();
      state.clear(); state.completed = false;
      form.elements.namedItem('body')?.focus();
    });
  }
  document.addEventListener('htmx:beforeOnLoad', event => {
    const xhr = event.detail.xhr;
    if (xhr.status < 200 || xhr.status >= 300 || !xhr.getResponseHeader('HX-Redirect')) return;
    const form = event.detail.elt?.closest('form[data-draft-key]');
    states.find(state => state.form === form)?.clear();
  });
  document.addEventListener('submit', event => {
    if (event.target.getAttribute('action') !== '/logout') return;
    for (const state of states) state.clear();
    try {
      for (const key of Object.keys(sessionStorage)) if (key.startsWith(prefix)) sessionStorage.removeItem(key);
    } catch {}
  });
  addEventListener('pagehide', () => { for (const state of states) state.save(); });
  addEventListener('beforeunload', event => {
    for (const state of states) state.save();
    if (states.some(state => state.unsaved)) { event.preventDefault(); event.returnValue = ''; }
  });
})();
