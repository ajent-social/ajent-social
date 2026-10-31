// Generated names remove naming work from consent. Server validation and
// ownership checks remain authoritative for every submitted value.
(() => {
 const form = document.querySelector('form[data-mcp-consent]');
 if (!form) return;
 const agent = form.querySelector('select[name="agent"]');
 const generated = form.querySelector('[data-generated-agent]');
 const status = form.querySelector('#connection-status');
 const update = () => { if (generated) generated.hidden = agent.value !== ''; };
 if (agent) { agent.addEventListener('change', update); update(); }
 let submitted = false;
 form.addEventListener('submit', event => {
  if (submitted) { event.preventDefault(); return; }
  submitted = true;
  // Keep the clicked submitter enabled until form data has been constructed:
  // disabling it synchronously would drop decision=approve/deny from the POST.
  if (status) status.textContent = 'Connecting… Please wait; do not click again.';
  form.setAttribute('aria-busy', 'true');
 });
 // Back/forward cache restoration must not strand an old disabled form.
 window.addEventListener('pageshow', () => { submitted = false; form.removeAttribute('aria-busy'); if (status) status.textContent = ''; });
})();
