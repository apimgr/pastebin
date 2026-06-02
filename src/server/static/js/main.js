// Pastebin — Main JavaScript
// Progressive enhancement only. All core features work without this file.

// ─── Service worker ───────────────────────────────────────────────────────────

if ('serviceWorker' in navigator) {
    window.addEventListener('load', async () => {
        try {
            const registration = await navigator.serviceWorker.register('/sw.js', { scope: '/' });

            // Detect when a new service worker version is waiting.
            registration.addEventListener('updatefound', () => {
                const newWorker = registration.installing;
                if (!newWorker) return;
                newWorker.addEventListener('statechange', () => {
                    if (newWorker.state === 'installed' && navigator.serviceWorker.controller) {
                        showUpdateBanner(registration);
                    }
                });
            });

            // Check for SW updates hourly when the app is active.
            setInterval(() => { registration.update(); }, 60 * 60 * 1000);
        } catch (_) {
            // Service worker unavailable — not an error condition.
        }
    });

    // Reload when a new SW takes control (after user clicks "Update Now").
    navigator.serviceWorker.addEventListener('controllerchange', () => {
        window.location.reload();
    });
}

// showUpdateBanner injects an update-available banner into the DOM.
function showUpdateBanner(registration) {
    if (document.getElementById('sw-update-banner')) return;
    const banner = document.createElement('div');
    banner.id = 'sw-update-banner';
    banner.setAttribute('role', 'status');
    banner.setAttribute('aria-live', 'polite');
    banner.style.cssText = [
        'position:fixed', 'bottom:1rem', 'right:1rem',
        'background:var(--accent,#6366f1)', 'color:#fff',
        'padding:0.75rem 1rem', 'border-radius:0.5rem',
        'display:flex', 'gap:0.5rem', 'align-items:center',
        'font-size:0.875rem', 'z-index:9999',
        'box-shadow:0 4px 12px rgba(0,0,0,.25)'
    ].join(';');
    banner.innerHTML = '<span>A new version is available.</span>' +
        '<button onclick="applyUpdate()" style="background:rgba(255,255,255,.2);border:none;color:inherit;padding:0.25rem 0.75rem;border-radius:0.25rem;cursor:pointer">Update Now</button>' +
        '<button onclick="this.parentElement.remove()" style="background:none;border:none;color:inherit;cursor:pointer;padding:0.25rem" aria-label="Dismiss">✕</button>';
    document.body.appendChild(banner);
    window.__swRegistration = registration;
}

// applyUpdate signals the waiting service worker to skip waiting.
function applyUpdate() {
    const reg = window.__swRegistration;
    if (reg && reg.waiting) {
        reg.waiting.postMessage({ type: 'SKIP_WAITING' });
    }
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
