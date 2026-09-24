// SPDX-License-Identifier: Apache-2.0
import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {createHandler} from './worker.mjs';
const snapshot = JSON.parse(await readFile(process.env.SKILLS_SNAPSHOT,'utf8'));
const handler = createHandler(snapshot);
const origin = new URL(snapshot.entries[0].result.content_url).origin;
const req = (path, init) => handler(new Request(origin+path,init));
const digest = text => createHash('sha256').update(text).digest('hex');

test('discovery, pinned raw bytes, attribution, conditional retrieval',async () => {
  const root = await req('/');
  const bootstrap = await root.text();
  assert.ok(bootstrap.startsWith('---\nname: ajent-skills'));
  assert.ok(!bootstrap.includes('{{ORIGIN}}'));
  assert.equal(digest(bootstrap),snapshot.bootstrap_sha256);
  const data = await (await req('/v1/skills/search?q=handoff')).json();
  assert.equal(data.schema,'ajent.skills.search/v1');
  assert.equal(data.results[0].id,'mattpocock-handoff');
  for (const entry of snapshot.entries) {
    const r = await handler(new Request(entry.result.content_url));
    assert.equal(r.status,200);
    assert.equal(digest(await r.text()),entry.result.version);
    assert.equal((await handler(new Request(entry.result.content_url,{headers:{'If-None-Match':r.headers.get('ETag')}}))).status,304);
    const notice = await handler(new Request(entry.result.license_url));
    assert.match(await notice.text(),/Copyright.*Matt Pocock/);
  }
});
test('request boundaries and no-match',async () => {
  for (const path of ['/v1/skills/search','/v1/skills/search?q=x&limit=','/v1/skills/search?q=x&limit=11','/v1/skills/search?q=x&limit=1e0','/v1/skills/search?q=x&q=y','/v1/skills/search?q=x&token=private-value','/v1/skills/search?q=%FF','/v1/skills/search?q=%ZZ','/v1/skills/search?q=!!!','/v1/skills/search?q='+ 'a'.repeat(2049)]) {
    const r=await req(path); assert.equal(r.status,400,path); assert.ok(!(await r.text()).includes('private-value'));
  }
  assert.equal((await req('/',{method:'POST'})).status,405);
  assert.equal(await (await req('/',{method:'HEAD'})).text(),'');
  assert.equal((await req('/v1/skills/missing/'+'0'.repeat(64))).status,404);
  assert.deepEqual((await (await req('/v1/skills/search?q=zznonexistent')).json()).results,[]);
});
test('edge and Go rank the fixture queries identically', {skip:!process.env.SKILLS_GO_ORIGIN}, async () => {
  for (const query of ['handoff','questionnaire','interview design','zznonexistent','CONTEXT handoff handoff','__proto__ constructor']) {
    const path='/v1/skills/search?q='+encodeURIComponent(query);
    const edge=await (await req(path)).json();
    const go=await (await fetch(process.env.SKILLS_GO_ORIGIN+path)).json();
    assert.deepEqual(edge.results.map(r=>[r.id,r.version]),go.results.map(r=>[r.id,r.version]),query);
  }
});
