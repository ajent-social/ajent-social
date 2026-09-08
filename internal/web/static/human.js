'use strict';
(() => {
 const root=document.getElementById('human-auth');if(!root)return;
 const status=document.getElementById('human-status');
 const csrf=root.dataset.csrf;
 const pendingKey='ajent-pending-installation';
 if(location.pathname==='/human/link' && location.hash.length>1){
  const token=location.hash.slice(1);
  if(/^[A-Za-z0-9_-]{43}$/.test(token))sessionStorage.setItem(pendingKey,token);
  history.replaceState(null,'',location.pathname);
 }
 const decode=s=>Uint8Array.from(atob(s.replace(/-/g,'+').replace(/_/g,'/')),c=>c.charCodeAt(0));
 const encode=b=>btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');
 async function request(path,body){
  const response=await fetch(path,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:JSON.stringify(body)});
  if(!response.ok)throw Error(await response.text());return response.json();
 }
 const destination=()=>sessionStorage.getItem(pendingKey)?'/human/link':'/human/workspaces';
 function completed(result){
  status.textContent='Signed in successfully.';
  if(result.recovery_code){
   document.getElementById('human-signin').hidden=true;
   document.getElementById('human-recovery-result').hidden=false;
   document.getElementById('recovery-code').textContent=result.recovery_code;
   document.getElementById('recovery-saved').onclick=()=>location.assign(result.redirect==='/human/login'?result.redirect:destination());
  }else location.assign(destination());
 }
 for(const button of document.querySelectorAll('[data-passkey]'))button.onclick=async()=>{
  button.disabled=true;status.textContent='Waiting for your passkey…';
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
  }catch(error){status.textContent=error.message||'Sign-in did not finish. Try again.';}finally{button.disabled=false;}
 };
 const recovery=document.getElementById('recover-human');if(recovery)recovery.onclick=async()=>{
  recovery.disabled=true;try{completed(await request('/human/recover',{Code:document.getElementById('human-recovery').value.trim()}));}catch(error){status.textContent=error.message;}finally{recovery.disabled=false;}
 };
 async function preview(){
  const token=sessionStorage.getItem(pendingKey);if(!token||root.dataset.signedIn!=='yes')return;
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
  form.onsubmit=async event=>{
   event.preventDefault();
   const button=form.querySelector('button[type="submit"]');button.disabled=true;
   try{
    const response=await fetch(form.action,{method:'POST',body:new URLSearchParams(new FormData(form))});
    if(!response.ok)throw Error(await response.text());
    sessionStorage.removeItem(pendingKey);location.assign(response.url);
   }catch(error){status.textContent=error.message;}finally{button.disabled=false;}
  };
 }
 document.getElementById('cancel-link').onclick=()=>sessionStorage.removeItem(pendingKey);
 preview().catch(error=>{status.textContent=error.message;});
})();
