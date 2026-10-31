// Navigate only to the server-authored, protocol-validated callback. Keep the
// visible link for browsers that block automatic navigation or disable scripts.
(() => {
 const link = document.querySelector('a#mcp-return');
 if (link) window.location.replace(link.href);
})();
