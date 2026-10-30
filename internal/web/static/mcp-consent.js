// Format validation is local only. The server still checks names, ownership,
// uniqueness, OAuth state and permissions when the human submits the form.
(() => {
  const form = document.querySelector('form[data-mcp-consent]');
  if (!form) return;
  const agent = form.querySelector('select[name="agent"]');
  const handle = form.querySelector('input[name="handle"]');
  const approve = form.querySelector('button[value="approve"]');
  const error = form.querySelector('#agent-handle-error');
  const help = form.querySelector('#agent-handle-help');
  if (!agent || !handle || !approve || !error || !help) return;
  let touched = false;
  const update = () => {
    const creating = agent.value === '';
    handle.disabled = !creating;
    handle.required = creating;
    const invalid = creating && (handle.value.length < handle.minLength ||
      handle.value.length > handle.maxLength || !handle.validity.valid);
    handle.setAttribute('aria-invalid', touched && invalid ? 'true' : 'false');
    error.textContent = touched && invalid ? `Invalid handle. ${help.textContent}` : '';
    approve.disabled = invalid;
  };
  handle.addEventListener('input', () => { touched = true; update(); });
  agent.addEventListener('change', update);
  update();
})();
