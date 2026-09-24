// Drafts stay in this tab and are removed after a confirmed server redirect.
(() => {
  const error = document.querySelector('#goal-error');
  if (error) error.focus();
  const pending = 'ajent-goal-pending';
  try {
    const key = sessionStorage.getItem(pending);
    if (key && new URLSearchParams(location.search).has('notice')) {
      sessionStorage.removeItem(key);
      sessionStorage.removeItem(pending);
    }
  } catch (_) { /* Storage may be disabled; native forms still work. */ }
  document.querySelectorAll('[data-goal-draft]').forEach(form => {
    const key = 'ajent-goal-draft:' + form.dataset.goalDraft;
    const fields = [...form.elements].filter(el => el.name && el.type !== 'hidden' && el.type !== 'submit');
    let dirty = false;
    try {
      const saved = JSON.parse(sessionStorage.getItem(key) || 'null');
      if (saved && !error) fields.forEach(el => { if (Object.hasOwn(saved, el.name)) el.value = saved[el.name]; });
      dirty = !!saved;
    } catch (_) { /* Ignore invalid or unavailable local drafts. */ }
    form.addEventListener('input', () => {
      dirty = true;
      try { sessionStorage.setItem(key, JSON.stringify(Object.fromEntries(fields.map(el => [el.name, el.value])))); } catch (_) {}
    });
    form.addEventListener('submit', () => {
      dirty = false;
      try { sessionStorage.setItem(pending, key); } catch (_) {}
    });
    form.addEventListener('keydown', event => {
      if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') form.requestSubmit();
    });
    window.addEventListener('beforeunload', event => { if (dirty) { event.preventDefault(); event.returnValue = ''; } });
  });
})();
