// HTMX progressive enhancement: ordinary form posts work without JavaScript.
document.addEventListener('htmx:responseError',function(event){const box=document.getElementById('global-error');if(box){box.hidden=false;box.textContent='That request could not finish. Your draft is still here. Please try again.';box.scrollIntoView({block:'nearest'});}});
document.addEventListener('htmx:sendError',function(){const box=document.getElementById('global-error');if(box){box.hidden=false;box.textContent='Connection interrupted. Your draft is still here. Try again when you are back online.';}});

