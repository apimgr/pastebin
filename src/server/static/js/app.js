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

// ─── Cookie consent & tracking (merged from consent.js) ───────────────────────

// Pastebin — Cookie Consent & Tracking (PART 31)
// Progressive enhancement only. Consent state lives entirely client-side in
// localStorage; the server never records per-visitor consent. Analytics scripts
// are shipped inert inside <template id="pb-tracking-snippet"> and only activated
// here once the visitor's analytics consent is known.
//
// The consent config itself is embedded by the server as an inert JSON
// script block (id="pb-consent-data") rather than an executable inline
// <script>, per project rules (no inline JS).

(function () {
    'use strict';

    var STORAGE_KEY = 'cookieConsent';
    var CCPA_KEY = 'ccpaDoNotSell';

    // loadConsentConfig parses the server-rendered JSON config block.
    // Returns {} when absent or unparsable, matching prior window.PB_CONSENT
    // fallback behavior.
    function loadConsentConfig() {
        var el = document.getElementById('pb-consent-data');
        if (!el) return {};
        try {
            return JSON.parse(el.textContent || '{}');
        } catch (e) {
            return {};
        }
    }

    var cfg = loadConsentConfig();

    // ─── Consent state ───────────────────────────────────────────────────────

    // readConsent returns the stored ConsentState, or null when absent/corrupt.
    function readConsent() {
        var raw = null;
        try {
            raw = localStorage.getItem(STORAGE_KEY);
        } catch (e) {
            return null;
        }
        if (!raw) {
            return null;
        }
        try {
            var c = JSON.parse(raw);
            if (typeof c !== 'object' || c === null) {
                return null;
            }
            return c;
        } catch (e) {
            try {
                localStorage.removeItem(STORAGE_KEY);
            } catch (e2) {
                // ignore
            }
            return null;
        }
    }

    // writeConsent persists the ConsentState and applies it immediately.
    function writeConsent(consent) {
        try {
            localStorage.setItem(STORAGE_KEY, JSON.stringify(consent));
        } catch (e) {
            // ignore
        }
        applyConsent(consent);
    }

    // nowSeconds returns the current UNIX time in seconds.
    function nowSeconds() {
        return Math.floor(Date.now() / 1000);
    }

    // ─── Tracking activation ─────────────────────────────────────────────────

    var trackingActivated = false;

    // activateTracking clones the inert tracking snippet into <head>, causing its
    // <script> elements to execute. Cloned template content does not run, so each
    // script node is recreated to trigger loading. Runs at most once.
    function activateTracking() {
        if (trackingActivated) {
            return;
        }
        var tpl = document.getElementById('pb-tracking-snippet');
        if (!tpl || !tpl.content) {
            return;
        }
        trackingActivated = true;
        var nodes = tpl.content.childNodes;
        for (var i = 0; i < nodes.length; i++) {
            var node = nodes[i];
            if (node.nodeType === 1 && node.tagName === 'SCRIPT') {
                var s = document.createElement('script');
                for (var a = 0; a < node.attributes.length; a++) {
                    var attr = node.attributes[a];
                    s.setAttribute(attr.name, attr.value);
                }
                s.text = node.textContent;
                document.head.appendChild(s);
            } else if (node.nodeType === 1) {
                document.head.appendChild(node.cloneNode(true));
            }
        }
    }

    // applyConsent enacts a ConsentState: preference cookie + analytics loading.
    function applyConsent(consent) {
        if (consent.preferences) {
            document.cookie = 'preferencesEnabled=true; path=/; max-age=31536000; samesite=lax';
        }
        if (consent.analytics && cfg.analyticsConfigured) {
            activateTracking();
        }
    }

    // ─── CCPA "Do Not Sell" ──────────────────────────────────────────────────

    // applyCCPAOptOut records the opt-out cookie and blocks data sharing.
    function applyCCPAOptOut() {
        document.cookie = 'ccpa_opt_out=true; path=/; max-age=31536000; samesite=lax';
    }

    // ccpaDoNotSell opts the visitor out of data sales and declines non-essential
    // cookies (CCPA right-to-opt-out).
    function ccpaDoNotSell() {
        try {
            localStorage.setItem(CCPA_KEY, 'true');
        } catch (e) {
            // ignore
        }
        applyCCPAOptOut();
        writeConsent({
            essential: true,
            preferences: false,
            analytics: false,
            ccpaOptOut: true,
            timestamp: nowSeconds()
        });
        hideBanner();
    }

    // ─── DOM helpers ─────────────────────────────────────────────────────────

    function el(tag, attrs, text) {
        var node = document.createElement(tag);
        if (attrs) {
            Object.keys(attrs).forEach(function (k) {
                node.setAttribute(k, attrs[k]);
            });
        }
        if (text != null) {
            node.textContent = text;
        }
        return node;
    }

    // ─── Consent banner ──────────────────────────────────────────────────────

    var bannerEl = null;

    function hideBanner() {
        if (bannerEl && bannerEl.parentNode) {
            bannerEl.parentNode.removeChild(bannerEl);
            bannerEl = null;
        }
    }

    function acceptAll() {
        writeConsent({
            essential: true,
            preferences: true,
            analytics: true,
            timestamp: nowSeconds()
        });
        hideBanner();
    }

    function declineAll() {
        writeConsent({
            essential: true,
            preferences: false,
            analytics: false,
            timestamp: nowSeconds()
        });
        hideBanner();
    }

    // buildBanner constructs and returns the consent banner element.
    function buildBanner() {
        var banner = el('div', {
            id: 'cookie-consent',
            class: 'cookie-banner cookie-banner-' + (cfg.position === 'top' ? 'top' : 'bottom'),
            role: 'region',
            'aria-label': 'Cookie consent',
            'data-sold': cfg.dataSold ? 'true' : 'false'
        });

        var content = el('div', { class: 'cookie-banner-content' });

        var message = el('span', { class: 'cookie-message' }, (cfg.message || '') + ' ');
        if (cfg.policyUrl) {
            message.appendChild(document.createTextNode('— '));
            message.appendChild(el('a', {
                href: cfg.policyUrl,
                class: 'policy-link'
            }, cfg.policyText || 'Privacy Policy'));
        }
        content.appendChild(message);

        var buttons = el('div', { class: 'cookie-buttons' });

        if (cfg.showPreferences) {
            var prefBtn = el('button', {
                type: 'button',
                class: 'btn-preferences'
            }, cfg.preferencesText || 'Manage Preferences');
            prefBtn.addEventListener('click', openPreferences);
            buttons.appendChild(prefBtn);
        }

        if (cfg.dataSold) {
            var dnsBtn = el('button', {
                type: 'button',
                class: 'btn-do-not-sell'
            }, 'Do Not Sell My Info');
            dnsBtn.addEventListener('click', ccpaDoNotSell);
            buttons.appendChild(dnsBtn);
        }

        var declineBtn = el('button', {
            type: 'button',
            class: 'btn-decline'
        }, cfg.declineText || 'Decline');
        declineBtn.addEventListener('click', declineAll);
        buttons.appendChild(declineBtn);

        var acceptBtn = el('button', {
            type: 'button',
            class: 'btn-accept'
        }, cfg.acceptText || 'Accept');
        acceptBtn.addEventListener('click', acceptAll);
        buttons.appendChild(acceptBtn);

        content.appendChild(buttons);
        banner.appendChild(content);
        return banner;
    }

    function showBanner() {
        if (bannerEl) {
            return;
        }
        bannerEl = buildBanner();
        document.body.appendChild(bannerEl);
    }

    // ─── Preferences modal ───────────────────────────────────────────────────

    var modalEl = null;

    function closePreferences() {
        if (modalEl && modalEl.parentNode) {
            modalEl.parentNode.removeChild(modalEl);
            modalEl = null;
        }
    }

    // categoryRow builds one cookie-category toggle row.
    function categoryRow(id, label, description, checked, locked) {
        var row = el('div', { class: 'cookie-category' });
        var head = el('label', { class: 'cookie-category-head', for: id });
        var input = el('input', { type: 'checkbox', id: id });
        if (checked) {
            input.checked = true;
        }
        if (locked) {
            input.checked = true;
            input.disabled = true;
        }
        head.appendChild(input);
        head.appendChild(el('span', { class: 'cookie-category-label' }, label));
        row.appendChild(head);
        if (description) {
            row.appendChild(el('p', { class: 'cookie-category-desc' }, description));
        }
        return row;
    }

    // openPreferences renders the granular preferences modal.
    function openPreferences() {
        if (modalEl) {
            return;
        }
        var current = readConsent() || {
            preferences: !!cfg.defaultPreferences,
            analytics: !!cfg.defaultAnalytics
        };
        var desc = cfg.descriptions || {};

        var overlay = el('div', {
            id: 'cookie-preferences-modal',
            class: 'cookie-modal-overlay',
            role: 'dialog',
            'aria-modal': 'true',
            'aria-label': cfg.preferencesText || 'Manage Preferences'
        });
        var dialog = el('div', { class: 'cookie-modal' });
        dialog.appendChild(el('h2', { class: 'cookie-modal-title' }, cfg.preferencesText || 'Manage Preferences'));

        dialog.appendChild(categoryRow('pref-essential', 'Essential',
            desc.essential || 'Required for the site to function.', true, true));
        dialog.appendChild(categoryRow('pref-preferences', 'Preferences',
            desc.preferences || 'Remember theme and language.', !!current.preferences, false));

        if (cfg.analyticsConfigured) {
            dialog.appendChild(categoryRow('pref-analytics', 'Analytics',
                desc.analytics || 'Anonymous usage statistics.', !!current.analytics, false));
        }

        var actions = el('div', { class: 'cookie-modal-actions' });
        var cancelBtn = el('button', { type: 'button', class: 'btn-decline' }, 'Cancel');
        cancelBtn.addEventListener('click', closePreferences);
        var saveBtn = el('button', { type: 'button', class: 'btn-accept' }, 'Save');
        saveBtn.addEventListener('click', savePreferences);
        actions.appendChild(cancelBtn);
        actions.appendChild(saveBtn);
        dialog.appendChild(actions);

        overlay.appendChild(dialog);
        overlay.addEventListener('click', function (ev) {
            if (ev.target === overlay) {
                closePreferences();
            }
        });
        document.addEventListener('keydown', escClose);
        modalEl = overlay;
        document.body.appendChild(overlay);
        saveBtn.focus();
    }

    function escClose(ev) {
        if (ev.key === 'Escape') {
            closePreferences();
            document.removeEventListener('keydown', escClose);
        }
    }

    // savePreferences persists the granular selections from the modal.
    function savePreferences() {
        var analyticsInput = document.getElementById('pref-analytics');
        var prefInput = document.getElementById('pref-preferences');
        writeConsent({
            essential: true,
            preferences: prefInput ? prefInput.checked : false,
            analytics: analyticsInput ? analyticsInput.checked : false,
            timestamp: nowSeconds()
        });
        closePreferences();
        hideBanner();
    }

    // ─── Init ────────────────────────────────────────────────────────────────

    function init() {
        // Honor a prior CCPA opt-out immediately.
        try {
            if (localStorage.getItem(CCPA_KEY) === 'true') {
                applyCCPAOptOut();
            }
        } catch (e) {
            // ignore
        }

        var consent = readConsent();
        if (consent) {
            applyConsent(consent);
        } else if (cfg.showUntilAcknowledged !== false) {
            showBanner();
        }

        // Expose the preferences modal so any page (e.g. /server/privacy) can
        // reopen it via a "Manage Preferences" control.
        window.pbShowCookiePreferences = openPreferences;

        // Wire any server-rendered "Manage Preferences" trigger.
        var triggers = document.querySelectorAll('[data-cookie-preferences]');
        for (var i = 0; i < triggers.length; i++) {
            triggers[i].addEventListener('click', function (ev) {
                ev.preventDefault();
                openPreferences();
            });
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();

// ─── Create page enhancements (merged from create.js) ─────────────────────────

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

// ─── Remove page enhancements (merged from remove.js) ─────────────────────────

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

// ─── Offline page reload button ───────────────────────────────────────────────
// Progressive enhancement only. Replaces the inline onclick handler on the
// offline page's "Try Again" button (no inline handlers per PART 16).
(function () {
    const btn = document.querySelector('[data-action="reload"]');
    if (btn) {
        btn.addEventListener('click', function () {
            window.location.reload();
        });
    }
})();
