// Explicit offline import: fetch the public catalog separately, then review this projection.
// node public-site/wiki/import-catalog.mjs INPUT.json FULL_COMMIT_SHA YYYY-MM-DD [--check]
import {readFileSync,writeFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import {projectCatalog} from './catalog.mjs';
const [input,revision,date,check] = process.argv.slice(2);
if (!input || !/^[a-f0-9]{40}$/.test(revision || '') || !/^\d{4}-\d{2}-\d{2}$/.test(date || '') || (check && check !== '--check')) throw new Error('Supply public catalog file, full revision and snapshot date');
const raw = readFileSync(input);
if (raw.length > 1024 * 1024) throw new Error('Catalog exceeds 1 MiB');
const source = {repository:'https://github.com/ajent-social/capabilities',revision,date,sha256:createHash('sha256').update(raw).digest('hex')};
const data = {source,capabilities:projectCatalog(JSON.parse(raw))};
const output = '// Generated from the reviewed public catalog. Run import-catalog.mjs; do not edit.\nexport const snapshot = ' + JSON.stringify(data,null,2) + ';\n';
const target = new URL('./snapshot.mjs',import.meta.url);
if (check === '--check') {
  if (readFileSync(target,'utf8') !== output) throw new Error('Snapshot is stale');
} else writeFileSync(target,output);
console.log(`${data.capabilities.length} public reference records ${check ? 'checked' : 'projected'} at ${revision}`);
