// Pastebin — Cookie Consent & Tracking (PART 31)
// Progressive enhancement only. Consent state lives entirely client-side in
// localStorage; the server never records per-visitor consent. Analytics scripts
// are shipped inert inside <template id="pb-tracking-snippet"> and only activated
// here once the visitor's analytics consent is known.

(function () {
    'use strict';

    var STORAGE_KEY = 'cookieConsent';
    var CCPA_KEY = 'ccpaDoNotSell';
    var cfg = window.PB_CONSENT || {};

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
