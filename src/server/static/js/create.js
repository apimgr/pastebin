// Pastebin — Create page enhancements
// Progressive enhancement only — the form works as a plain POST without any
// of this. These handlers add convenience for JS-capable browsers.
(function () {
    const TOKEN_KEY = 'pastebin_owner_token';

    // Persist a freshly issued owner token (rendered server-side after a
    // successful create) so the next paste can reuse it.
    const createdToken = document.getElementById('created-token');
    if (createdToken && createdToken.textContent.trim()) {
        localStorage.setItem(TOKEN_KEY, createdToken.textContent.trim());
    }

    // Pre-fill the token field from a previously saved token.
    const tokenField = document.getElementById('owner_token');
    if (tokenField && !tokenField.value) {
        const saved = localStorage.getItem(TOKEN_KEY) || '';
        if (saved) {
            tokenField.value = saved;
        }
    }

    // Copy-token button (server-rendered result block).
    const copyBtn = document.querySelector('[data-copy-target]');
    if (copyBtn && navigator.clipboard) {
        copyBtn.addEventListener('click', function () {
            const el = document.getElementById(this.dataset.copyTarget);
            if (!el) return;
            navigator.clipboard.writeText(el.textContent.trim());
            const original = this.textContent;
            this.textContent = 'Copied!';
            setTimeout(() => { this.textContent = original; }, 1500);
        });
    }

    // Tab key inserts spaces in the textarea instead of moving focus.
    const content = document.getElementById('content');
    if (content) {
        content.addEventListener('keydown', function (e) {
            if (e.key === 'Tab') {
                e.preventDefault();
                const start = this.selectionStart;
                const end = this.selectionEnd;
                this.value = this.value.substring(0, start) + '    ' + this.value.substring(end);
                this.selectionStart = this.selectionEnd = start + 4;
            }
        });
        content.focus();
    }
})();
