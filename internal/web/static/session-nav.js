// Keep the public return link in sync with this browser's current account.
(()=>{'use strict';
 const links=[...document.querySelectorAll('[data-session-link]')];
 if(!links.length)return;
 const joins=[...document.querySelectorAll('[data-session-join]')];
 let loading=false,again=false;
 function update(state){
  if(typeof state.human!=='boolean'||typeof state.agent!=='boolean')return;
  const destination=state.human?'/human/workspaces':state.agent?'/app':'/human/login';
  const label=state.human?'Your workspaces':state.agent?'Agent dashboard':'Sign in';
  for(const link of links){link.href=destination;link.textContent=label;}
  for(const join of joins)join.textContent=state.human||state.agent?'Connect agent':'Join ajent';
 }
 async function refresh(){
  if(document.hidden)return;
  if(loading){again=true;return;}
  loading=true;
  do{
   again=false;
   const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),5000);
   try{
    const response=await fetch('/session-status',{credentials:'same-origin',cache:'no-store',signal:controller.signal});
    if(response.ok)update(await response.json());
   }catch{/* Keep the usable return link when the status request is unavailable. */}
   finally{clearTimeout(timer);}
  }while(again&&!document.hidden);
  loading=false;
 }
 window.addEventListener('pageshow',refresh);
 window.addEventListener('focus',refresh);
 document.addEventListener('visibilitychange',()=>{if(!document.hidden)refresh();});
 if(document.readyState==='complete')refresh();
})();
