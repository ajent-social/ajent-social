(() => {
  const tag = document.currentScript;
  const id = tag?.dataset.measurementId;
  if (!/^G-[A-Za-z0-9]+$/.test(id || '')) return;
  window.dataLayer = window.dataLayer || [];
  window.gtag = window.gtag || function () { window.dataLayer.push(arguments); };
  window.gtag('js', new Date());
  const pageUrl = value => {
    try {
      const url = new URL(value);
      return url.origin + url.pathname;
    } catch {
      return '';
    }
  };
  window.gtag('config', id, {
    page_location: pageUrl(window.location.href),
    page_referrer: pageUrl(document.referrer),
  });
  const script = document.createElement('script');
  script.async = true;
  script.src = 'https://www.googletagmanager.com/gtag/js?id=' + encodeURIComponent(id);
  document.head.append(script);
})();
