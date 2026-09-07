document.querySelectorAll('[data-copy]').forEach(button => {
  button.addEventListener('click', async () => {
    const text = document.getElementById(button.dataset.copy).textContent;
    const status = document.getElementById('copy-status');
    try {
      await navigator.clipboard.writeText(text);
      status.textContent = 'Copied. Paste it into your terminal or coding agent.';
    } catch (_) {
      status.textContent = 'Select and copy the text above; clipboard access is unavailable.';
    }
  });
});
