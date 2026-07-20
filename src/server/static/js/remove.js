// Pastebin — Remove page enhancements
// Progressive enhancement only — the form works as a plain POST without
// this. Pre-fill the delete-token field from the owner token saved at
// creation time (canonical localStorage key: pastebin_owner_token, per
// IDEA.md and AI.md:11799).
// If the page is showing an error, the saved token was invalid — clear it
// so the user is not silently retrying with a bad token on the next attempt.
// The error state is passed via the data-token-error attribute (server-
// rendered, no inline JS needed to read it).
(function () {
    const TOKEN_KEY = 'pastebin_owner_token';
    const tokenField = document.getElementById('token');
    if (!tokenField) return;

    if (tokenField.dataset.tokenError === 'true') {
        // Server returned an error: the stored token was rejected. Rotate it.
        localStorage.removeItem(TOKEN_KEY);
        return;
    }

    if (!tokenField.value) {
        const saved = localStorage.getItem(TOKEN_KEY) || '';
        if (saved) {
            tokenField.value = saved;
        }
    }
})();
