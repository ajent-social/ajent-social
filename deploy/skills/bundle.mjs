// SPDX-License-Identifier: Apache-2.0
// Usage: node deploy/skills/bundle.mjs validated-snapshot.json output.mjs
import {readFile, writeFile} from 'node:fs/promises';
const [input, output] = process.argv.slice(2);
if (!input || !output) throw new Error('Specify input snapshot and output module');
const snapshot = JSON.parse(await readFile(input, 'utf8'));
const adapter = await readFile(new URL('./worker.mjs',import.meta.url),'utf8');
await writeFile(output, adapter + '\nconst catalog = ' + JSON.stringify(snapshot) + ';\nexport default {fetch:createHandler(catalog)};\n');
