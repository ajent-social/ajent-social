import {test} from 'node:test';
import assert from 'node:assert/strict';
import {handle,APP,PUBLIC} from './worker.mjs';
test('public reads strip account secrets and preserve query',async()=>{
 let called=false;
 const r=await handle(new Request(PUBLIC+'/domains?q=tools',{headers:{Cookie:'private',Authorization:'Bearer secret','X-Forwarded-For':'spoof',Accept:'text/html'}}),async(url,init)=>{called=true;assert.equal(url,APP+'/domains?q=tools');assert.equal(init.headers.get('Cookie'),null);assert.equal(init.headers.get('Authorization'),null);assert.equal(init.headers.get('X-Forwarded-For'),null);return new Response('<html><head></head><body>public</body></html>',{headers:{'Content-Type':'text/html','Set-Cookie':'private=value'}})});
 assert.ok(called);assert.equal(r.headers.get('Set-Cookie'),null);assert.equal(r.headers.get('Cache-Control'),'no-store');assert.match(await r.text(),/canonical.*https:\/\/ajent.social\/domains/);
});
test('sign-in and credential routes stay on the app origin',async()=>{
 for(const path of ['/app','/app/verification','/login','/connect-login','/v1/me','/v1/signup','/install.sh','/skill.md']){const r=await handle(new Request(PUBLIC+path),()=>{throw Error('must not proxy')});assert.equal(r.status,307);assert.equal(r.headers.get('Location'),APP+path);}
});
test('canonical host and HTTPS redirect',async()=>{for(const url of ['http://ajent.social/docs','https://www.ajent.social/docs']){const r=await handle(new Request(url));assert.equal(r.status,308);assert.equal(r.headers.get('Location'),PUBLIC+'/docs')}});
test('chat requires the public browser origin and strips credentials',async()=>{
 assert.equal((await handle(new Request(PUBLIC+'/api/chat',{method:'POST',headers:{Origin:'https://other.example'}}))).status,403);
 let called=false;const r=await handle(new Request(PUBLIC+'/api/chat',{method:'POST',headers:{Origin:PUBLIC,Authorization:'secret'},body:'{"messages":[]}'}),async(url,init)=>{called=true;assert.equal(init.headers.get('Origin'),APP);assert.equal(init.headers.get('Authorization'),null);return Response.json({answer:'Guide'})});assert.ok(called);assert.equal(r.status,200);
});
test('unknown mutations are not proxied',async()=>assert.equal((await handle(new Request(PUBLIC+'/domains',{method:'POST'}))).status,405));
test('upstream failures remain recoverable',async()=>{const r=await handle(new Request(PUBLIC+'/'),()=>{throw Error('offline')});assert.equal(r.status,503);assert.match(await r.text(),/try again/)});
test('metadata changes do not change the documented API origin',async()=>{const r=await handle(new Request(PUBLIC+'/docs'),async()=>new Response('<head><link rel="canonical" href="'+APP+'/docs"></head><p>AJENT_URL='+APP+'</p>',{headers:{'Content-Type':'text/html'}}));const html=await r.text();assert.ok(html.includes(PUBLIC+'/docs'));assert.ok(html.includes('AJENT_URL='+APP))});
test('sitemap URLs use the public domain',async()=>{const r=await handle(new Request(PUBLIC+'/sitemap.xml'),async()=>new Response('<loc>'+APP+'/docs</loc>'));assert.equal(await r.text(),'<loc>'+PUBLIC+'/docs</loc>')});

test('public agent instructions identify the authenticated API origin',async()=>{for(const path of ['/llms.txt','/']){const r=await handle(new Request(PUBLIC+path),async()=>new Response("Use this site's origin as AJENT_URL.",{headers:{'Content-Type':'text/markdown'}}));assert.equal(await r.text(),'Use '+APP+' as AJENT_URL.')}});
