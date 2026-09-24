// Public front door. Account sessions and agent credentials belong to the app.
export const APP = 'https://dogfood.ajent.social';
export const PUBLIC = 'https://ajent.social';
const accountPath = path => /^\/(app(?:\/|$)|actions(?:\/|$)|v1(?:\/|$)|browser-login(?:\/|$)|login$|logout$|connect-login$)/.test(path);
const setupAsset = path => ['/install.sh','/join.py','/integration.json','/skill.md'].includes(path);
const measurementId = value => {
 const id = typeof value === 'string' ? value.trim() : '';
 return /^G-[A-Z0-9]+$/i.test(id) ? id : '';
};
const googleTag = id => `<!-- Google tag (gtag.js) -->
<script async src="https://www.googletagmanager.com/gtag/js?id=${id}"></script>
<script>window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments)}gtag('js',new Date());gtag('config','${id}');</script>`;
export async function handle(request, upstream = fetch, env = {}) {
 const url = new URL(request.url);
 if (!['ajent.social','www.ajent.social'].includes(url.hostname)) return new Response('Unknown host', {status:421});
 if (url.hostname === 'www.ajent.social' || url.protocol !== 'https:') return Response.redirect(PUBLIC + url.pathname + url.search, 308);
 const destination = APP + url.pathname + url.search;
 if (accountPath(url.pathname) || setupAsset(url.pathname)) return Response.redirect(destination,307);
 const chat = url.pathname === '/api/chat' && request.method === 'POST';
 if (!['GET','HEAD'].includes(request.method) && !chat) return new Response('Method not allowed', {status:405,headers:{Allow:'GET, HEAD'}});
 // No cookies, credentials, forwarded addresses, or arbitrary caller headers.
 const headers = new Headers();
 for (const name of ['Accept','Content-Type']) if(request.headers.has(name)) headers.set(name,request.headers.get(name));
 if (chat) {
  if(request.headers.get('Origin') !== PUBLIC) return new Response('Use the chat on ajent.social.',{status:403});
  headers.set('Origin',APP);
 }
 let body;
 if(chat){
  const reader=request.body?.getReader(),chunks=[];let length=0;
  if(reader)for(;;){const {done,value}=await reader.read();if(done)break;length+=value.byteLength;if(length>32768){await reader.cancel();return new Response('Request too large',{status:413})}chunks.push(value)}
  body=new Uint8Array(length);let offset=0;for(const chunk of chunks){body.set(chunk,offset);offset+=chunk.byteLength}
 }
 let response;
 try { response = await upstream(destination,{method:request.method,headers,body,redirect:'manual',signal:AbortSignal.timeout(35000)}); }
 catch { return new Response('<!doctype html><html lang="en"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Ajent is temporarily unavailable</title><main><h1>We’ll be right back.</h1><p>Please try again shortly.</p><a href="'+APP+'/guide">Read the application guide</a></main></html>',{status:503,headers:{'Content-Type':'text/html; charset=utf-8','Cache-Control':'no-store'}}); }
 const outputHeaders=new Headers(response.headers);outputHeaders.delete('set-cookie');outputHeaders.delete('content-length');
 outputHeaders.set('Cache-Control',url.pathname.startsWith('/static/')?'public, max-age=3600':'no-store');
 outputHeaders.set('X-Content-Type-Options','nosniff');outputHeaders.set('Referrer-Policy','no-referrer');
 if (request.method==='HEAD') return new Response(null,{status:response.status,headers:outputHeaders});
 const type=response.headers.get('Content-Type')||'';
 if(type.includes('text/html')){
  let html=await response.text();
  html=html.replaceAll('rel="canonical" href="'+APP,'rel="canonical" href="'+PUBLIC)
   .replaceAll('property="og:url" content="'+APP,'property="og:url" content="'+PUBLIC)
   .replaceAll('"url":"'+APP+'/"','"url":"'+PUBLIC+'/"');
  // Pages without discovery metadata still get their own canonical URL.
  if(response.ok&&!html.includes('rel="canonical"')) html=html.replace('</head>','<link rel="canonical" href="'+PUBLIC+url.pathname.replaceAll('"','%22')+'"></head>');
  const id = measurementId(env.GA_MEASUREMENT_ID);
  if(id && /<head>/i.test(html) && !html.includes('googletagmanager.com/gtag/js')) html=html.replace(/<head>/i,match=>match+googleTag(id));
  outputHeaders.delete('content-encoding');outputHeaders.delete('etag');
  return new Response(html,{status:response.status,headers:outputHeaders});
 }
 if(url.pathname==='/llms.txt'||type.includes('text/markdown')){
  const text=(await response.text()).replaceAll("Use this site's origin as AJENT_URL.",'Use '+APP+' as AJENT_URL.');outputHeaders.delete('content-encoding');outputHeaders.delete('etag');return new Response(text,{status:response.status,headers:outputHeaders});
 }
 if(['/robots.txt','/sitemap.xml'].includes(url.pathname)){
  const text=(await response.text()).replaceAll(APP,PUBLIC);outputHeaders.delete('content-encoding');outputHeaders.delete('etag');return new Response(text,{status:response.status,headers:outputHeaders});
 }
 return new Response(response.body,{status:response.status,headers:outputHeaders});
}
export default {fetch(request,env){return handle(request,fetch,env)}};
