// Deterministic static rendering for the native Go application. No network access.
import {readFileSync,writeFileSync} from 'node:fs';
import {snapshot} from './snapshot.mjs';
import {css} from './style.mjs';
import {handleWiki,card,shell,snapshotNote,callout} from './wiki.mjs';
const check=process.argv.includes('--check');
const dest=new URL('../../internal/web/wiki/',import.meta.url);
const externalCSS=html=>html.replace(`<style>${css}</style>`,'<link rel="stylesheet" href="/wiki/style.css">');
const pages={};
for(const path of ['/wiki',...snapshot.capabilities.map(c=>'/wiki/amsl/'+c.id)]) {
  pages[path]=externalCSS(await handleWiki(new Request('https://ajent.social'+path)).text());
}
pages['/wiki']=pages['/wiki'].replace(`Browse ${snapshot.capabilities.length} capability records`,'Browse capability records');
const bundle={source:snapshot.source,capabilities:snapshot.capabilities,pages,cards:Object.fromEntries(snapshot.capabilities.map(c=>[c.id,card(c)]))};
const body=readFileSync(new URL('./catalog-body.html',import.meta.url),'utf8')+snapshotNote()+callout();
const template=externalCSS(shell('AMSL capability catalog','Reusable contracts, existing alternatives and verification evidence.','/wiki/amsl',body)).replace('</head>','{{if .Filtered}}<meta name="robots" content="noindex,follow">{{end}}</head>');
for(const [name,value] of Object.entries({'bundle.json':JSON.stringify(bundle,null,2)+'\n','catalog.html':template,'style.css':css})) {
 const path=new URL(name,dest);
 if(check){if(readFileSync(path,'utf8')!==value)throw Error(name+' is stale');}else writeFileSync(path,value);
}
console.log('Native wiki assets '+(check?'checked':'generated'));
