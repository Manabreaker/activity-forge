// Capture OAuth handoff before rendering or loading anything else. No token in history.
(() => {
  const hash = new URLSearchParams(location.hash.slice(1));
  const session = hash.get('session');
  if (!session) return;
  history.replaceState(null, '', location.pathname + location.search);
  try {
    const expected = sessionStorage.getItem('af.oauth-nonce');
    sessionStorage.removeItem('af.oauth-nonce');
    if (!expected || hash.get('nonce') !== expected || !/^[\w-]{40,16000}$/.test(session)) return;
    sessionStorage.setItem('af.session', session);
  } catch { /* The editor works even when browser storage is unavailable. */ }
})();
