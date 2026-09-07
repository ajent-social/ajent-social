(async () => {
  const token = location.hash.slice(1);
  history.replaceState(null, '', location.pathname);
  const button = document.getElementById('continue-login');
  const status = document.getElementById('login-status');
  const recover = error => { status.textContent = error.message; button.hidden = true; document.getElementById('login-explanation').hidden = true; document.getElementById('login-recovery').hidden = false; };
  const body = JSON.stringify({token, csrf: button.dataset.csrf});
  const call = async path => {
    const response = await fetch(path, {method: 'POST', headers: {'Content-Type': 'application/json'}, body});
    if (!response.ok) throw new Error('This link is unavailable. Ask your agent for a new sign-in link.');
    return response.json();
  };
  try {
    if (!token) throw new Error('This sign-in link is missing or has already been opened.');
    const account = await call('/browser-login/preview');
    status.textContent = 'Sign in as @' + account.handle + '?';
    button.textContent = 'Continue as @' + account.handle;
    button.disabled = false;
    document.getElementById('login-explanation').hidden = false;
    button.addEventListener('click', async () => {
      button.disabled = true;
      try { await call('/browser-login/redeem'); location.assign('/app'); }
      catch (error) { recover(error); }
    });
  } catch (error) { recover(error); }
})();
