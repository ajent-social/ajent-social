// SPDX-License-Identifier: Apache-2.0
// HTTP edge adapter for snapshots validated and exported by skillsd.
export function createHandler(catalog) {
  if (catalog.schema !== 'ajent.skills.edge/v1') throw new Error('Invalid snapshot');
  const entries = catalog.entries;
  const byKey = new Map(entries.map(e => [e.result.id + '/' + e.result.version, e]));
  const tokens = text => text.toLowerCase().match(/[\p{L}\p{Nd}]+/gu) || [];
  const bytes = text => new TextEncoder().encode(text).length;
  function search(query, limit) {
    const terms = [...new Set(tokens(query))];
    return entries.map((entry, index) => {
      let score = 0;
      for (const term of terms) {
        const tf = Object.hasOwn(entry.terms, term) ? entry.terms[term] : 0;
        if (!tf) continue;
        const df = catalog.document_frequency[term];
        const idf = Math.log(1 + (entries.length - df + 0.5) / (df + 0.5));
        score += idf * tf * 2.2 / (tf + 1.2 * (0.25 + 0.75 * entry.length / catalog.average_length));
      }
      return {entry, index, score};
    }).filter(e => e.score > 0).sort((a,b) => b.score - a.score || a.index - b.index)
      .slice(0,limit).map(e => e.entry.result);
  }
  return async function fetch(request) {
    const url = new URL(request.url);
    const headers = new Headers({'X-Content-Type-Options':'nosniff', 'Referrer-Policy':'no-referrer', 'Cache-Control':'no-store'});
    const respond = (body, status=200) => new Response(request.method === 'HEAD' || status === 304 ? null : body, {status, headers});
    const json = (value, status=200) => {
      headers.set('Content-Type', 'application/json; charset=utf-8');
      return respond(JSON.stringify(value) + '\n', status);
    };
    const error = (status, code, message) => json({error:{code,message}}, status);
    const text = (body, digest, type, immutable=false) => {
      headers.set('Content-Type', type + '; charset=utf-8');
      headers.set('ETag', '"' + digest + '"');
      headers.set('Cache-Control', immutable ? 'public, max-age=31536000, immutable' : 'public, max-age=300');
      if (request.headers.get('If-None-Match') === headers.get('ETag')) return respond(null,304);
      return respond(body);
    };
    if (!['GET','HEAD'].includes(request.method)) {
      headers.set('Allow','GET, HEAD');
      return error(405,'method_not_allowed','Use GET or HEAD.');
    }
    if (bytes(url.pathname + url.search) > 8192) return error(414,'invalid_query','Request URL is too long.');
    if (url.pathname === '/' || url.pathname === '/SKILL.md') return text(catalog.bootstrap,catalog.bootstrap_sha256,'text/markdown');
    if (url.pathname === '/healthz') return json({status:'ok',catalog_version:catalog.catalog_version,skills:entries.length});
    if (url.pathname === '/v1/skills/search') {
      // URLSearchParams replaces malformed UTF-8; reject it before parsing.
      try { decodeURIComponent(url.search.replace(/\+/g,' ')); }
      catch { return error(400,'invalid_query','Invalid query encoding.'); }
      if (url.search.includes(';')) return error(400,'invalid_query','Invalid query encoding.');
      const params = url.searchParams;
      for (const key of params.keys()) {
        if (!['q','limit'].includes(key) || params.getAll(key).length !== 1) return error(400,'invalid_query','Use one q and optional limit.');
      }
      const query = (params.get('q') || '').trim();
      if (!query || bytes(query) > 2048 || !tokens(query).length) return error(400,'invalid_query','q must contain searchable text, at most 2048 bytes.');
      const raw = params.get('limit');
      const limit = raw === null ? 5 : Number(raw);
      if ((raw !== null && !/^\+?\d+$/.test(raw)) || !Number.isInteger(limit) || limit < 1 || limit > 10) return error(400,'invalid_query','limit must be between 1 and 10.');
      return json({schema:'ajent.skills.search/v1',catalog_version:catalog.catalog_version,results:search(query,limit)});
    }
    let path;
    try { path = decodeURIComponent(url.pathname); } catch { return error(404,'not_found','Resource not found.'); }
    const isLicense = path.startsWith('/v1/licenses/');
    const prefix = isLicense ? '/v1/licenses/' : '/v1/skills/';
    const entry = path.startsWith(prefix) ? byKey.get(path.slice(prefix.length)) : null;
    if (!entry) return error(404,'not_found','Skill version not found.');
    if (isLicense) return text(entry.license,entry.license_sha256,'text/plain');
    headers.set('X-Content-SHA256',entry.result.content_sha256);
    return text(entry.content,entry.result.content_sha256,'text/markdown',true);
  };
}
