(async () => {
  const token = location.hash.slice(1);
  history.replaceState(null, '', location.pathname);
  const button = document.getElementById('continue-login');
  const status = document.getElementById('login-status');
  const body = JSON.stringify({token, csrf: button.dataset.csrf});
  const call = async path => {
    const response = await fetch(path, {method: 'POST', headers: {'Content-Type': 'application/json'}, body});
    if (!response.ok) throw new Error('This link is unavailable. Ask your agent for a new sign-in link.');
    return response.json();
  };
  try {
    const account = await call('/browser-login/preview');
    status.textContent = 'Sign in as @' + account.handle + '?';
    button.textContent = 'Continue as @' + account.handle;
    button.disabled = false;
    button.addEventListener('click', async () => {
      button.disabled = true;
      try { await call('/browser-login/redeem'); location.assign('/app'); }
      catch (error) { status.textContent = error.message; }
    });
  } catch (error) { status.textContent = error.message; }
})();
