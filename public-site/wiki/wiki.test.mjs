import {test} from 'node:test';
import assert from 'node:assert/strict';
const PUBLIC='https://ajent.social';
import {handleWiki,wikiPaths} from './wiki.mjs';
import {projectCatalog} from './catalog.mjs';
import {snapshot} from './snapshot.mjs';
const noUpstream = () => {throw new Error('Wiki must not retrieve authenticated or remote content');};
const request = (path,init) => new Request(PUBLIC+path,init);
const read = (path,env={},init) => handleWiki(request(path,init),env);

test('wiki and every capability are public without upstream access',async()=>{
 for(const path of wikiPaths()){
  const response=await read(path,{}, {headers:{Cookie:'private=session',Authorization:'Bearer private'}});
  assert.equal(response.status,200,path);assert.equal(response.headers.get('Cache-Control'),'no-store');assert.equal(response.headers.get('Set-Cookie'),null);
  const body=await response.text();assert.match(body,/<html lang="en">/);assert.match(body,/<h1>/);assert.match(body,/rel="canonical"/);assert.doesNotMatch(body,/private=session|Bearer private/);
  assert.equal((body.match(/<h1>/g)||[]).length,1,path);
 }
});
test('candidate detail distinguishes proposal from verification',async()=>{
 const body=await (await read('/wiki/amsl/identity.service-credentials')).text();
 for(const text of ['Proposed guarantees','No AMSL implementation is recorded','No executed verification or real consumer adoption','restricted, maintainer-reported','9ea4343'])assert.ok(body.includes(text),text);
 assert.ok(body.includes('https://github.com/ajent-social/capabilities/blob/'+snapshot.source.revision+'/capabilities/identity.service-credentials.json'));
});
test('rejected capabilities retain their useful external alternatives',async()=>{
 const body=await (await read('/wiki/amsl/protocol.mcp-server')).text();
 assert.match(body,/rejected/);assert.match(body,/Use an existing solution/);assert.match(body,/Official MCP Go SDK/);
});
test('search and domain filters work without JavaScript',async()=>{
 const body=await (await read('/wiki/amsl?q=credential&domain=identity')).text();
 assert.match(body,/Scoped service credentials/);assert.doesNotMatch(body,/Customer linkage and checkout/);assert.match(body,/name="robots" content="noindex,follow"/);assert.match(body,/action="\/wiki\/amsl" method="get"/);
 const empty=await (await read('/wiki/amsl?q=no-matching-record')).text();assert.match(empty,/No matching capabilities/);
});
test('search query is escaped in HTML and cannot add active markup',async()=>{
 const q='\"><script>alert(1)</script>';
 const response=await read('/wiki/amsl?q='+encodeURIComponent(q));const body=await response.text();
 assert.doesNotMatch(body,/<script>/);assert.match(body,/&lt;script&gt;/);assert.match(response.headers.get('Content-Security-Policy'),/default-src 'none'/);
});
test('invalid domains and oversized queries fail explicitly',async()=>{
 for(const path of ['/wiki/amsl?domain=__proto__','/wiki/amsl?domain=private','/wiki/amsl?q='+'a'.repeat(161)])assert.equal((await read(path)).status,400);
});
test('unknown, unpublished and traversal-like wiki paths return 404',async()=>{
 for(const path of ['/wiki/private','/wiki/amsl/not-real','/wiki/go','/wiki/amsl/identity.service-credentials/extra','/wiki/amsl/%3Cscript%3E']){
  const response=await read(path);assert.equal(response.status,404,path);assert.equal(response.headers.get('X-Robots-Tag'),'noindex');
 }
});
test('HEAD preserves status and headers but returns no body',async()=>{
 for(const path of ['/wiki','/wiki/amsl','/wiki/amsl/catalog.json','/wiki/sitemap.xml','/wiki/unknown']){
  const get=await read(path);const head=await read(path,{}, {method:'HEAD'});assert.equal(head.status,get.status);assert.equal(head.headers.get('Content-Type'),get.headers.get('Content-Type'));assert.equal(await head.text(),'');
 }
});
test('mutations are denied and trailing slash redirects are canonical',async()=>{
 const response=await read('/wiki/amsl',{}, {method:'POST'});assert.equal(response.status,405);assert.equal(response.headers.get('Allow'),'GET, HEAD');
 const redirect=await read('/wiki/amsl/?q=keys');assert.equal(redirect.status,308);assert.equal(redirect.headers.get('Location'),PUBLIC+'/wiki/amsl?q=keys');
});
test('withdrawal suppresses detail, search, JSON, both sitemaps and lists',async()=>{
 const id='identity.service-credentials',env={WIKI_HIDDEN_IDS:id};
 assert.equal((await read('/wiki/amsl/'+id,env)).status,404);
 for(const path of ['/wiki/amsl','/wiki/amsl?q=credentials','/wiki/amsl/catalog.json','/wiki/sitemap.xml'])assert.ok(!(await (await read(path,env)).text()).includes(id),path);

});
test('emergency disable removes discovery and returns unavailable without content',async()=>{
 const env={WIKI_ENABLED:'false'};assert.deepEqual(wikiPaths(env),[]);
 for(const path of ['/wiki','/wiki/amsl','/wiki/amsl/catalog.json','/wiki/sitemap.xml']){const r=await read(path,env);assert.equal(r.status,503);assert.doesNotMatch(await r.text(),/Scoped service credentials/)}

});
test('machine projection excludes restricted evidence bodies and reports provenance',async()=>{
 const response=await read('/wiki/amsl/catalog.json');const data=await response.json();
 assert.equal(data.source.revision,snapshot.source.revision);assert.equal(data.capabilities.length,12);
 assert.ok(data.capabilities.every(c=>!Object.hasOwn(c,'evidence')));assert.ok(data.capabilities.some(c=>c.restrictedEvidence));
});

function rawRecord(){return {id:'identity.example',domain:'identity',name:'Example',status:'CANDIDATE',disposition:'EXTRACT',summary:'Example contract',use_when:['Needed'],non_goals:['Everything'],proposed_guarantees:['Proposed'],failure_modes:['Failure'],open_questions:['Question'],alternatives:[{name:'Library',url:'https://example.com',assessment:'Evaluate'}],implementations:[],verification:[],evidence:[{visibility:'RESTRICTED',basis:'CONFIDENTIAL BODY',url:'https://private.example/source'}],private_notes:'DO NOT COPY'};}
test('projection never copies restricted evidence or unknown fields',()=>{
 const projected=JSON.stringify(projectCatalog({schema_version:1,capabilities:[rawRecord()]}));
 assert.doesNotMatch(projected,/CONFIDENTIAL|private.example|DO NOT COPY|private_notes/);assert.match(projected,/restrictedEvidence/);
});
test('projection rejects malformed records and unsafe links',()=>{
 for(const mutate of [r=>r.id='../../escape',r=>r.domain='__proto__',r=>r.status='APPROVED',r=>r.alternatives[0].url='javascript:alert(1)',r=>r.alternatives[0].url='https://user:secret@example.com',r=>r.implementations=[{url:'http://example.com',revision:'abc'}],r=>r.use_when='wrong']){
  const record=rawRecord();mutate(record);assert.throws(()=>projectCatalog({schema_version:1,capabilities:[record]}));
 }
 assert.throws(()=>projectCatalog({schema_version:1,capabilities:[rawRecord(),rawRecord()]}));
});
test('non-wiki paths are left to the existing application handler',()=>assert.equal(handleWiki(request('/guide')),null));
