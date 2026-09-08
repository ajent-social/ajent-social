/* Conversation and pending-bubble pattern adapted from sirelog/sire-chat.
 * Model output stays plain text; source links come from server-validated IDs. */
(()=>{'use strict';
const dialog=document.getElementById('ajent-chat');if(!dialog||!dialog.show)return;
const launcher=document.getElementById('chat-launcher'),input=document.getElementById('chat-question'),thread=document.getElementById('chat-conversation'),send=document.getElementById('chat-send'),suggestions=document.getElementById('chat-suggestions');
let history=[],controller=null,generation=0,opener=null;
const dock=document.querySelector('.chat-dock');dock.hidden=false;document.body.classList.add('has-chat');
function resizeInput(){input.style.height='auto';input.style.height=input.scrollHeight+'px';}
input.addEventListener('input',resizeInput);
function open(event){opener=event?.currentTarget||(document.activeElement===document.body?launcher:document.activeElement);if(!dialog.open)dialog.showModal();launcher.hidden=true;input.focus();resizeInput();}
launcher.hidden=false;launcher.addEventListener('click',open);
document.querySelectorAll('[data-open-chat]').forEach(b=>{b.hidden=false;b.addEventListener('click',e=>{e.preventDefault();open(e)})});
document.getElementById('chat-close').addEventListener('click',()=>dialog.close());
dialog.addEventListener('close',()=>{launcher.hidden=false;(opener?.isConnected?opener:launcher).focus()});
document.addEventListener('keydown',e=>{if(e.key==='Escape'&&dialog.open){e.preventDefault();dialog.close()}});
function bubble(role,text){const row=document.createElement('div');row.className='chat-message '+role;const box=document.createElement('div');box.className='chat-bubble';box.textContent=text;row.append(box);thread.append(row);thread.scrollTop=thread.scrollHeight;return box;}
async function ask(question){if(controller)return;const current=++generation;controller=new AbortController();const activeController=controller;const timeout=setTimeout(()=>activeController.abort(),32000);send.disabled=true;if(input.value.trim()===question)input.value='';resizeInput();if(dialog.open)input.focus();suggestions.hidden=true;bubble('user',question);const pending=bubble('ai','Looking through the Ajent guide…');pending.setAttribute('aria-busy','true');
let prior=history.slice(-6);while(prior.reduce((n,m)=>n+m.content.length,0)+question.length>11000)prior=prior.slice(2);
try{const response=await fetch('/api/chat',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({messages:[...prior,{role:'user',content:question}]}),signal:controller.signal});const data=await response.json();if(current!==generation)return;if(!response.ok)throw new Error(data.error||'Chat is unavailable. Please try again.');pending.textContent=data.answer;const label=document.createElement('small');label.className='chat-mode';label.textContent=data.mode==='ai'?'AI answer · Check the sources':'Published guide · AI unavailable';pending.append(label);if(Array.isArray(data.citations)){const links=document.createElement('div');links.className='chat-sources';for(const cite of data.citations){if(typeof cite.url!=='string'||!/^\/guide#[a-z0-9-]+$/.test(cite.url))continue;const a=document.createElement('a');a.href=cite.url;a.textContent=cite.title;links.append(a);}pending.append(links);}history=[...prior,{role:'user',content:question},{role:'assistant',content:data.answer}];}
catch(err){if(current!==generation)return;pending.textContent=err.name==='AbortError'?'The answer took too long. Try again or read the guide.':(err instanceof SyntaxError?'Chat is temporarily unavailable. Try again or read the guide.':err.message);if(!input.value.trim())input.value=question;resizeInput();const retry=document.createElement('button');retry.type='button';retry.className='chat-retry';retry.textContent='Try this question again';retry.addEventListener('click',()=>{if(controller)return;retry.disabled=true;ask(question)});pending.append(retry);}
finally{clearTimeout(timeout);if(current===generation){controller=null;send.disabled=false;pending.removeAttribute('aria-busy');thread.scrollTop=thread.scrollHeight;}}}
document.getElementById('chat-form').addEventListener('submit',e=>{e.preventDefault();const q=input.value.trim();if(q)ask(q)});
input.addEventListener('keydown',e=>{if(e.key==='Enter'&&!e.shiftKey&&!e.isComposing){e.preventDefault();document.getElementById('chat-form').requestSubmit();}});
suggestions.querySelectorAll('button').forEach(b=>b.addEventListener('click',()=>ask(b.dataset.question)));
document.getElementById('chat-clear').addEventListener('click',()=>{if((controller||history.length||input.value.trim())&&!confirm('Start a new chat? This clears the conversation and your unfinished question.'))return;generation++;controller?.abort();controller=null;history=[];thread.replaceChildren();bubble('ai','A fresh start. What would you like to know about Ajent?');suggestions.hidden=false;send.disabled=false;input.value='';resizeInput();input.focus();});
const url=new URL(location.href);if(url.searchParams.get('ask')==='1'){open();url.searchParams.delete('ask');window.history.replaceState(null,'',url.pathname+url.search+url.hash);}
})();
