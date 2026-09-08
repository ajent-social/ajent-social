'use strict';
(() => {
 document.addEventListener('submit',async event=>{
  const form=event.target;
  const message=event.submitter?.dataset.confirm||form.dataset.confirm;
  if(message&&!window.confirm(message)){event.preventDefault();return;}
  if(!form.hasAttribute('data-enhance'))return;
  event.preventDefault();if(form.dataset.busy==='yes')return;
  const body=new URLSearchParams(new FormData(form));
  if(event.submitter?.name)body.append(event.submitter.name,event.submitter.value);
  const buttons=[...form.querySelectorAll('button')];
  let feedback=form.querySelector('[data-form-status]');
  if(!feedback){feedback=document.createElement('p');feedback.dataset.formStatus='';feedback.className='form-status';feedback.setAttribute('role','status');feedback.tabIndex=-1;form.append(feedback);}
  form.dataset.busy='yes';form.setAttribute('aria-busy','true');
  for(const button of buttons)button.disabled=true;
  feedback.textContent='Saving…';
  try{
   const response=await fetch(form.getAttribute('action'),{method:'POST',body});
   if(!response.ok){
    const detail=response.headers.get('Content-Type')?.startsWith('text/plain')?(await response.text()).trim():'';
    feedback.textContent=detail||'This change could not be saved. Try again.';
    if(response.status===401){const link=document.createElement('a');link.href='/human/login';link.textContent='Sign in again';feedback.append(' ',link);}
    feedback.focus();return;
   }
   location.assign(response.url);
  }catch{feedback.textContent='The request did not finish. Try again.';feedback.focus();}
  finally{delete form.dataset.busy;form.removeAttribute('aria-busy');for(const button of buttons)button.disabled=false;}
 });
 const root=document.getElementById('human-auth');if(!root)return;
 const status=document.getElementById('human-status');
 const csrf=root.dataset.csrf;
 const pendingKey='ajent-pending-installation';
 let pendingToken='',pendingStored=false;
 try{pendingToken=sessionStorage.getItem(pendingKey)||'';pendingStored=!!pendingToken;}catch{}
 if(!/^[A-Za-z0-9_-]{43}$/.test(pendingToken)){pendingToken='';pendingStored=false;}
 function clearPending(){pendingToken='';pendingStored=false;try{sessionStorage.removeItem(pendingKey);}catch{}}
 if(['/human/link','/human/login'].includes(location.pathname) && location.hash.length>1){
  const token=location.hash.slice(1);
  if(/^[A-Za-z0-9_-]{43}$/.test(token)){
   pendingToken=token;pendingStored=false;
   try{sessionStorage.setItem(pendingKey,token);pendingStored=true;}catch{}
  }
  history.replaceState(null,'',location.pathname);
 }
 const decode=s=>Uint8Array.from(atob(s.replace(/-/g,'+').replace(/_/g,'/')),c=>c.charCodeAt(0));
 const encode=b=>btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');
 async function request(path,body){
  const response=await fetch(path,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:JSON.stringify(body)});
  if(!response.ok){const error=Error(await response.text());error.status=response.status;throw error;}return response.json();
 }
 // The fragment is never sent to the server. The query forces a page load when
 // returning to this same link page; capture above immediately removes both.
 const resume=path=>path+(pendingToken&&!pendingStored?'?resume=1#'+pendingToken:'');
 const destination=()=>pendingToken?resume('/human/link'):'/human/workspaces';
 function completed(result){
  status.textContent='Signed in successfully.';
  if(result.recovery_code){
   document.getElementById('human-signin').hidden=true;
   document.getElementById('human-recovery-result').hidden=false;
   document.getElementById('recovery-saved').focus();
   document.getElementById('recovery-code').textContent=result.recovery_code;
   document.getElementById('recovery-saved').onclick=()=>location.assign(result.redirect==='/human/login'?resume(result.redirect):destination());
  }else location.assign(destination());
 }
 let busy=false;
 const authButtons=[...root.querySelectorAll('[data-passkey],#recover-human')];
 function setBusy(value){busy=value;for(const button of authButtons)button.disabled=value;document.getElementById('human-signin').setAttribute('aria-busy',String(value));}
 function failed(error,field){
  status.textContent=error.name==='NotAllowedError'?'The passkey step was cancelled or timed out. Try again when you are ready.':error.message||'Sign-in did not finish. Try again.';
  if(field){field.setAttribute('aria-invalid','true');field.focus();}else status.focus();
 }
 for(const field of root.querySelectorAll('input'))field.addEventListener('input',()=>field.removeAttribute('aria-invalid'));
 async function passkey(button){
  if(busy)return;
  setBusy(true);status.textContent='Waiting for your passkey…';
  try{
   if(!window.PublicKeyCredential)throw Error('This browser does not support passkeys. Use a current browser on HTTPS.');
   const action=button.dataset.passkey;
   const options=await request('/human/passkeys/begin',{Action:action,Name:document.getElementById('human-name')?.value||''});
   const publicKey=options.publicKey;
   publicKey.challenge=decode(publicKey.challenge);
   if(publicKey.user)publicKey.user.id=decode(publicKey.user.id);
   for(const item of publicKey.excludeCredentials||[])item.id=decode(item.id);
   for(const item of publicKey.allowCredentials||[])item.id=decode(item.id);
   const credential=action==='register'?await navigator.credentials.create({publicKey}):await navigator.credentials.get({publicKey});
   if(!credential)throw Error('No passkey was selected. Try again.');
   const response={clientDataJSON:encode(credential.response.clientDataJSON)};
   if(action==='register'){
    response.attestationObject=encode(credential.response.attestationObject);
    response.transports=credential.response.getTransports?.()||[];
   }else{
    response.authenticatorData=encode(credential.response.authenticatorData);
    response.signature=encode(credential.response.signature);
    response.userHandle=credential.response.userHandle?encode(credential.response.userHandle):null;
   }
   completed(await request('/human/passkeys/finish',{id:credential.id,rawId:encode(credential.rawId),type:credential.type,response,clientExtensionResults:credential.getClientExtensionResults()}));
  }catch(error){failed(error,error.status===400?document.getElementById('human-name'):null);}finally{setBusy(false);}
 }
 for(const button of root.querySelectorAll('[data-passkey]')){
  if(button.form)button.form.addEventListener('submit',event=>{event.preventDefault();passkey(button);});
  else button.addEventListener('click',()=>passkey(button));
 }
 const recovery=document.getElementById('human-recover');if(recovery)recovery.addEventListener('submit',async event=>{
  event.preventDefault();if(busy)return;
  setBusy(true);status.textContent='Checking your recovery code…';
  const field=document.getElementById('human-recovery');
  try{completed(await request('/human/recover',{Code:field.value.trim()}));field.value='';}catch(error){failed(error,error.status===401?field:null);}finally{setBusy(false);}
 });
 async function preview(){
  const token=pendingToken;if(!token||root.dataset.signedIn!=='yes')return;
  const result=await request('/human/link/preview',{Token:token});
  const form=document.getElementById('installation-form');
  form.elements.csrf.value=result.csrf;form.elements.token.value=token;
  document.getElementById('installation-description').textContent=(result.existing?'Claim existing installation: ':'Approve new installation: ')+result.handle;
  document.getElementById('installation-code').textContent=result.code;
  const select=document.getElementById('link-workspace');
  if(!result.existing)select.add(new Option('Create a new workspace',''));
  for(const workspace of result.workspaces)select.add(new Option(workspace.name,workspace.id));
  const change=()=>{
   document.getElementById('new-workspace-label').hidden=select.value!=='';
   const domain=document.getElementById('link-domain');domain.replaceChildren(new Option('Private mode — no domain affiliation',''));
   for(const name of result.domains[select.value]||[])domain.add(new Option(name,name));
  };
  select.onchange=change;change();
  document.getElementById('installation-consent').hidden=false;
  select.focus();
  form.onsubmit=async event=>{
   event.preventDefault();
   const button=form.querySelector('button[type="submit"]');button.disabled=true;
   try{
    const response=await fetch(form.getAttribute('action'),{method:'POST',body:new URLSearchParams(new FormData(form))});
    if(!response.ok)throw Error(await response.text());
    clearPending();location.assign(response.url);
   }catch(error){status.textContent=error.message;}finally{button.disabled=false;}
  };
 }
 document.getElementById('cancel-link').onclick=clearPending;
 preview().catch(error=>{status.textContent=error.message;});
})();
