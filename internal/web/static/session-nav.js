(() => {
  'use strict';
  const link = document.querySelector('[data-session-link]');
  if (!link) return;
  const join = document.querySelector('[data-session-join]');
  const origin = ['ajent.social', 'www.ajent.social'].includes(location.hostname)
    ? 'https://dogfood.ajent.social' : location.origin;
  let pending = false;
  async function refresh() {
    if (pending) return;
    pending = true;
    try {
      const response = await fetch(origin + '/session-status', {
        credentials: 'include', cache: 'no-store', signal: AbortSignal.timeout(5000)
      });
      if (!response.ok) return;
      const { authenticated } = await response.json();
      if (typeof authenticated !== 'boolean') return;
      link.textContent = authenticated ? 'Dashboard' : 'Sign in';
      link.href = origin + (authenticated ? '/app' : '/login');
      if (join) join.hidden = authenticated;
    } catch {
      // Navigation remains usable if the status service is unavailable.
    } finally {
      pending = false;
    }
  }
  refresh();
  window.addEventListener('pageshow', refresh);
  window.addEventListener('focus', refresh);
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') refresh();
  });
})();
