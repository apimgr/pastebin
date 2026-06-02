// Pastebin — Main JavaScript
// Progressive enhancement only. All core features work without this file.

// ─── Service worker ───────────────────────────────────────────────────────────
if ('serviceWorker' in navigator) {
    window.addEventListener('load', () => {
        navigator.serviceWorker.register('/sw.js').catch(() => {});
    });
}

// ─── Theme ────────────────────────────────────────────────────────────────────

// Apply persisted user theme preference on load.
// Only overrides when the user has explicitly chosen (never for "auto" — CSS handles it).
document.addEventListener('DOMContentLoaded', () => {
    const saved = localStorage.getItem('theme');
    if (saved === 'dark' || saved === 'light') {
        document.documentElement.setAttribute('data-theme', saved);
    }
});

function toggleTheme() {
    const html = document.documentElement;
    const current = html.getAttribute('data-theme');
    const next = current === 'light' ? 'dark' : 'light';
    html.setAttribute('data-theme', next);
    localStorage.setItem('theme', next);
}

// ─── Copy to clipboard ────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
    document.querySelectorAll('[data-copy]').forEach(btn => {
        btn.addEventListener('click', () => {
            const text = btn.getAttribute('data-copy');
            if (!text) return;

            const copyText = btn.querySelector('.copy-text');

            navigator.clipboard.writeText(text).then(() => {
                btn.classList.add('copied');
                if (copyText) copyText.textContent = 'Copied!';
                setTimeout(() => {
                    btn.classList.remove('copied');
                    if (copyText) copyText.textContent = 'Copy';
                }, 2000);
            }).catch(() => {
                // Fallback: select the adjacent code element
                const block = btn.closest('.code-block');
                const code = block && block.querySelector('.code-content');
                if (code) {
                    const range = document.createRange();
                    range.selectNodeContents(code);
                    const sel = window.getSelection();
                    sel.removeAllRanges();
                    sel.addRange(range);
                }
            });
        });
    });
});

// ─── Submit button loading state ─────────────────────────────────────────────

// Per PART 16: disable submit on click, show loading text, re-enable on response.
document.addEventListener('DOMContentLoaded', () => {
    document.querySelectorAll('form').forEach(form => {
        form.addEventListener('submit', () => {
            const btn = form.querySelector('[type="submit"]');
            if (!btn || btn.disabled) return;

            const originalText = btn.textContent.trim();
            const loadingMap = {
                'create': 'Creating…',
                'save': 'Saving…',
                'submit': 'Submitting…',
                'delete': 'Deleting…',
                'send': 'Sending…',
                'search': 'Searching…',
                'upload': 'Uploading…',
            };
            const lower = originalText.toLowerCase();
            const loadingText = loadingMap[lower] || originalText + '…';

            btn.disabled = true;
            btn.style.minWidth = btn.offsetWidth + 'px';
            btn.textContent = loadingText;
        });
    });
});

// ─── API helper ───────────────────────────────────────────────────────────────

async function fetchAPI(endpoint, options = {}) {
    const defaults = {
        headers: {
            'Content-Type': 'application/json',
            'Accept': 'application/json',
        },
    };
    const response = await fetch(endpoint, { ...defaults, ...options });
    if (!response.ok) {
        const err = await response.json().catch(() => ({}));
        throw new Error(err.detail || err.error || 'API error');
    }
    return response.json();
}
