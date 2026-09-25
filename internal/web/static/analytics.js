(() => {
  const tag = document.currentScript;
  const id = tag?.dataset.measurementId;
  if (!/^G-[A-Za-z0-9]+$/.test(id || '')) return;
  window.dataLayer = window.dataLayer || [];
  window.gtag = window.gtag || function () { window.dataLayer.push(arguments); };
  window.gtag('js', new Date());
  const publicPaths = new Set([
    '/', '/about', '/ask', '/directory', '/explore', '/guide', '/join',
    '/privacy', '/start', '/terms', '/wiki', '/docs',
  ]);
  const routePath = path => {
    if (path === '/docs' || path.startsWith('/docs/')) return '/docs';
    if (path === '/wiki' || path.startsWith('/wiki/')) return '/wiki';
    return publicPaths.has(path) ? path : '/public-page';
  };
  const pageUrl = value => {
    try {
      const url = new URL(value);
      if (url.origin !== window.location.origin) return url.origin;
      return url.origin + routePath(url.pathname);
    } catch {
      return window.location.origin + '/public-page';
    }
  };
  const pageLocation = pageUrl(window.location.href);
  window.gtag('config', id, {
    page_location: pageLocation,
    page_referrer: document.referrer ? pageUrl(document.referrer) : '',
    page_title: pageLocation.slice(window.location.origin.length),
  });
  const script = document.createElement('script');
  script.async = true;
  script.src = 'https://www.googletagmanager.com/gtag/js?id=' + encodeURIComponent(id);
  document.head.append(script);
})();
