// Optional local preview launcher. Production uses the Go binary directly.
import {spawn} from 'node:child_process';
import {readFileSync,existsSync} from 'node:fs';
if(existsSync('.env'))for(const line of readFileSync('.env','utf8').split('\n')){const m=line.match(/^([A-Z_]+)=(.*)$/);if(m&&!process.env[m[1]])process.env[m[1]]=m[2];}
let qaDB,qaServer;
if(process.env.AJENT_EMBEDDED_QA_DB==='1'){
 const {PGlite}=await import('@electric-sql/pglite');
 const {pg_trgm}=await import('@electric-sql/pglite/contrib/pg_trgm');
 const {PGLiteSocketServer}=await import('@electric-sql/pglite-socket');
 qaDB=await PGlite.create({extensions:{pg_trgm}});
 qaServer=new PGLiteSocketServer({db:qaDB,host:'127.0.0.1',port:55432,maxConnections:1});
 await qaServer.start();
}
const args=process.argv.slice(2);let port='8080',host='127.0.0.1';
for(let i=0;i<args.length;i++){if(args[i]==='--port')port=args[++i];if(args[i]==='--host')host=args[++i];}
const serve=['-addr',`${host}:${port}`,...(process.env.AJENT_DEMO==='1'?['-seed-demo']:[])];
const binary=process.env.AJENT_BINARY;
const child=spawn(binary||process.env.AJENT_GO||'go',binary?serve:['run','-buildvcs=false','./cmd/ajentd',...serve],{stdio:'inherit',env:process.env});
child.on('error',err=>{console.error(err.message);process.exit(1)});
process.on('SIGTERM',()=>child.kill('SIGTERM'));process.on('SIGINT',()=>child.kill('SIGINT'));child.on('exit',code=>process.exit(code??0));
