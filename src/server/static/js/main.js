// Pastebin — Main JavaScript
// Progressive enhancement only. All core features work without this file.

// ─── i18n ─────────────────────────────────────────────────────────────────────

// English fallback strings for the few messages this script generates at
// runtime. The server embeds the active-locale bundle as an inert JSON
// script block (id="pb-i18n-data") rather than an executable inline
// <script>, per project rules (no inline JS). This map is used when that
// bundle is absent or a key is missing (e.g. cached page, no template).
const I18N_FALLBACK = {
    update_available: 'A new version is available.',
    update_now: 'Update Now',
    dismiss: 'Dismiss',
    copied: 'Copied!',
    copy: 'Copy',
    api_error: 'API error',
    creating: 'Creating…',
    saving: 'Saving…',
    submitting: 'Submitting…',
    deleting: 'Deleting…',
    sending: 'Sending…',
    searching: 'Searching…',
    uploading: 'Uploading…',
    working: '…',
    offline: 'You are offline',
};

// loadI18nBundle parses the server-rendered <script type="application/json"
// id="pb-i18n-data"> block into a plain object. Returns {} when the block is
// absent or unparsable, so callers always fall back to I18N_FALLBACK.
function loadI18nBundle() {
    const el = document.getElementById('pb-i18n-data');
    if (!el) return {};
    try {
        return JSON.parse(el.textContent || '{}');
    } catch (_) {
        return {};
    }
}

const PB_I18N = loadI18nBundle();

// t returns the localized string for key, falling back to English.
function t(key) {
    return PB_I18N[key] || I18N_FALLBACK[key] || key;
}

// ─── Toast Notifications ─────────────────────────────────────────────────────

// showToast displays a transient notification in the #toast-container element.
// type: 'info' | 'success' | 'warning' | 'error'
function showToast(message, type) {
    var toastType = type || 'info';
    var container = document.getElementById('toast-container');
    if (!container) {
        container = document.createElement('div');
        container.id = 'toast-container';
        document.body.appendChild(container);
    }
    var toast = document.createElement('div');
    toast.className = 'toast toast--' + toastType;
    toast.setAttribute('role', 'status');
    toast.setAttribute('aria-live', 'polite');
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(function () {
        toast.classList.add('toast--dismissing');
        toast.addEventListener('animationend', function () {
            if (toast.parentNode) toast.parentNode.removeChild(toast);
        });
    }, 3000);
}

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

// showUpdateBanner injects an update-available banner into the DOM using CSS classes.
function showUpdateBanner(registration) {
    if (document.getElementById('sw-update-banner')) return;
    const banner = document.createElement('div');
    banner.id = 'sw-update-banner';
    banner.className = 'sw-update-banner';
    banner.setAttribute('role', 'status');
    banner.setAttribute('aria-live', 'polite');

    const msg = document.createElement('span');
    msg.textContent = t('update_available');

    const applyBtn = document.createElement('button');
    applyBtn.className = 'sw-update-banner__btn-apply';
    applyBtn.textContent = t('update_now');
    applyBtn.addEventListener('click', applyUpdate);

    const dismissBtn = document.createElement('button');
    dismissBtn.className = 'sw-update-banner__btn-dismiss';
    dismissBtn.textContent = '✕';
    dismissBtn.setAttribute('aria-label', t('dismiss'));
    dismissBtn.addEventListener('click', () => banner.remove());

    banner.appendChild(msg);
    banner.appendChild(applyBtn);
    banner.appendChild(dismissBtn);
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

// ─── Offline Detection ───────────────────────────────────────────────────────

// Toggle the #offline-indicator element based on network connectivity.
function updateOfflineIndicator() {
    const indicator = document.getElementById('offline-indicator');
    if (!indicator) return;
    if (navigator.onLine) {
        indicator.hidden = true;
    } else {
        indicator.hidden = false;
    }
}

window.addEventListener('online', updateOfflineIndicator);
window.addEventListener('offline', updateOfflineIndicator);

document.addEventListener('DOMContentLoaded', updateOfflineIndicator);

// ─── Theme ────────────────────────────────────────────────────────────────────

// Source of truth is the server-readable `theme` cookie rendered as the class on
// <html> (PART 16). No preference is read from localStorage. Without JS, the
// header <form> POSTs to /theme, sets the cookie, and reloads. This enhancement
// intercepts the submit to cycle the theme in place — no reload, no FOUC.

// themeCycle advances dark → light → auto → dark, matching nextTheme() on the server.
function themeCycle(current) {
    if (current === 'dark') return 'light';
    if (current === 'light') return 'auto';
    return 'dark';
}

// currentTheme reads the active mode from the class on <html> (theme-dark by default).
function currentTheme() {
    const cls = document.documentElement.className || '';
    const m = cls.match(/theme-(dark|light|auto)/);
    return m ? m[1] : 'dark';
}

document.addEventListener('DOMContentLoaded', () => {
    const form = document.querySelector('form.theme-toggle');
    if (!form) return;
    form.addEventListener('submit', (e) => {
        e.preventDefault();
        const next = themeCycle(currentTheme());
        document.documentElement.className = `theme-${next}`;
        document.cookie = `theme=${next}; path=/; max-age=31536000; SameSite=Lax`;
        const hidden = form.querySelector('input[name="theme"]');
        if (hidden) hidden.value = themeCycle(next);
    });
});

// ─── Copy: code blocks (home page quick-start) ───────────────────────────────

// copyCode copies the <pre> text in the nearest .code-block ancestor.
// Called via data-copy-code attribute buttons.
function copyCode(btn) {
    const block = btn.closest('.code-block');
    if (!block) return;
    const pre = block.querySelector('pre');
    if (!pre) return;
    navigator.clipboard.writeText(pre.textContent).then(() => {
        const orig = btn.textContent;
        btn.textContent = t('copied');
        setTimeout(() => { btn.textContent = orig || t('copy'); }, 2000);
    }).catch(() => {
        // Fallback: select the text.
        const range = document.createRange();
        range.selectNodeContents(pre);
        const sel = window.getSelection();
        sel.removeAllRanges();
        sel.addRange(range);
    });
}

// copyToClipboard copies the text content of the element with the given id.
// Called via data-copy-paste attribute buttons.
function copyToClipboard(elementId) {
    const el = document.getElementById(elementId || 'paste-code');
    const btn = document.querySelector('[data-copy-paste]');
    if (!el) return;
    navigator.clipboard.writeText(el.textContent).then(() => {
        if (btn) {
            const orig = btn.textContent;
            btn.textContent = t('copied');
            setTimeout(() => { btn.textContent = orig; }, 2000);
        }
    }).catch(() => {
        if (btn) {
            btn.textContent = 'Failed';
            setTimeout(() => { btn.textContent = t('copy'); }, 2000);
        }
    });
}

// ─── Copy to clipboard (data-copy attribute) ─────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
    document.querySelectorAll('[data-copy]').forEach(btn => {
        btn.addEventListener('click', () => {
            const text = btn.getAttribute('data-copy');
            if (!text) return;

            const copyText = btn.querySelector('.copy-text');

            navigator.clipboard.writeText(text).then(() => {
                btn.classList.add('copied');
                if (copyText) copyText.textContent = t('copied');
                setTimeout(() => {
                    btn.classList.remove('copied');
                    if (copyText) copyText.textContent = t('copy');
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

// ─── data-copy-code buttons ───────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
    document.querySelectorAll('[data-copy-code]').forEach(btn => {
        btn.addEventListener('click', () => copyCode(btn));
    });
});

// ─── data-copy-paste buttons ─────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
    document.querySelectorAll('[data-copy-paste]').forEach(btn => {
        btn.addEventListener('click', () => {
            const targetId = btn.getAttribute('data-copy-paste');
            copyToClipboard(targetId);
        });
    });
});

// ─── QR download ─────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
    const dlBtn = document.querySelector('[data-qr-download]');
    if (!dlBtn) return;
    dlBtn.addEventListener('click', () => {
        const src = dlBtn.getAttribute('data-qr-download');
        const name = dlBtn.getAttribute('data-qr-name') || 'paste-qr.png';
        const a = document.createElement('a');
        a.href = src;
        a.download = name;
        a.click();
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
                'create': t('creating'),
                'save': t('saving'),
                'submit': t('submitting'),
                'delete': t('deleting'),
                'send': t('sending'),
                'search': t('searching'),
                'upload': t('uploading'),
            };
            const lower = originalText.toLowerCase();
            const loadingText = loadingMap[lower] || originalText + t('working');

            btn.disabled = true;
            btn.style.minWidth = btn.offsetWidth + 'px';
            btn.textContent = loadingText;
        });
    });
});

// ─── API helper ───────────────────────────────────────────────────────────────

async function fetchAPI(endpoint, options) {
    const opts = options || {};
    const defaults = {
        headers: {
            'Content-Type': 'application/json',
            'Accept': 'application/json',
        },
    };
    const response = await fetch(endpoint, Object.assign({}, defaults, opts));
    if (!response.ok) {
        const err = await response.json().catch(() => ({}));
        throw new Error(err.detail || err.error || t('api_error'));
    }
    return response.json();
}
