/* =========================================================================
 *  BDPV6 Launcher — frontend bootstrap
 *
 *  Stack: vanilla ES2020, no framework, no build step. The Wails runtime
 *  injects window.go.main.App at startup; bound Go methods are then
 *  callable as window.go.main.App.<Method>(...).
 *
 *  This file establishes the SPA shell (nav + view registry). For F1 only
 *  the Dashboard view is implemented; other tabs render a placeholder so
 *  the navigation skeleton is in place for F2-F8 to plug into.
 * ========================================================================= */

/* ---------- ICONS ----------------------------------------------------------
 *  Lucide icons (ISC license) inlined as SVG path strings. Names match the
 *  Lucide catalogue so they're easy to swap. Keep this file as the single
 *  source of truth for icons used in the UI.
 * ------------------------------------------------------------------------- */
const ICONS = {
    cpu:        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="16" height="16" x="4" y="4" rx="2"/><rect width="6" height="6" x="9" y="9" rx="1"/><path d="M15 2v2M15 20v2M2 15h2M2 9h2M20 15h2M20 9h2M9 2v2M9 20v2"/></svg>',
    layout:     '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2"/><path d="M3 9h18M9 21V9"/></svg>',
    server:     '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="20" height="8" x="2" y="2" rx="2" ry="2"/><rect width="20" height="8" x="2" y="14" rx="2" ry="2"/><line x1="6" x2="6.01" y1="6" y2="6"/><line x1="6" x2="6.01" y1="18" y2="18"/></svg>',
    plug:       '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22v-5M9 7V2M15 7V2M6 13h12M6 7h12v6a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V7z"/></svg>',
    terminal:   '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" x2="20" y1="19" y2="19"/></svg>',
    folder:     '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>',
    wrench:     '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>',
    book:       '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H20v20H6.5a2.5 2.5 0 0 1 0-5H20"/></svg>',
    settings:   '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>',
    alert:      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><line x1="12" x2="12" y1="9" y2="13"/><line x1="12" x2="12.01" y1="17" y2="17"/></svg>',
    check:      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>',
    info:       '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg>',
};

function iconHTML(name) {
    return ICONS[name] || ICONS.info;
}

/* ---------- i18n (STR) ---------------------------------------------------- */
const STR = {
    es: {
        'brand.subtitle':   'Launcher',
        'nav.dashboard':    'Dashboard',
        'nav.services':     'Servicios',
        'nav.ports':        'Puertos',
        'nav.consoles':     'Consolas',
        'nav.hdfs':         'HDFS',
        'nav.repair':       'Reparar',
        'nav.notebooks':    'Notebooks',
        'nav.settings':     'Configuración',
        'view.dashboard.title':   'Dashboard',
        'view.services.title':    'Servicios',
        'view.ports.title':       'Puertos',
        'view.consoles.title':    'Consolas de diagnóstico',
        'view.hdfs.title':        'Explorador HDFS',
        'view.repair.title':      'Reparar',
        'view.notebooks.title':   'Notebooks',
        'view.settings.title':    'Configuración',
        'dashboard.envInfo':      'Información del entorno',
        'dashboard.layout':       'Distribución BDP detectada',
        'dashboard.setup':        'Estado de configuración',
        'dashboard.missing':      'Faltan carpetas en la distribución:',
        'dashboard.layoutOK':     'Distribución BDP detectada correctamente. Todas las carpetas de servicio están presentes junto al launcher.',
        'dashboard.setupPending': 'La configuración inicial aún no se ha ejecutado. El asistente arrancará la próxima vez que inicies los servicios (disponible en la fase F5).',
        'dashboard.setupDone':    'Configuración inicial completada.',
        'dashboard.namenodeMissing':'NameNode HDFS no formateado.',
        'placeholder.upcoming':   'Esta vista se entrega en una fase posterior del plan.',
        'version.line':           'v{0} · {1}/{2}',
    },
    en: {
        'brand.subtitle':   'Launcher',
        'nav.dashboard':    'Dashboard',
        'nav.services':     'Services',
        'nav.ports':        'Ports',
        'nav.consoles':     'Consoles',
        'nav.hdfs':         'HDFS',
        'nav.repair':       'Repair',
        'nav.notebooks':    'Notebooks',
        'nav.settings':     'Settings',
        'view.dashboard.title':   'Dashboard',
        'view.services.title':    'Services',
        'view.ports.title':       'Ports',
        'view.consoles.title':    'Diagnostic consoles',
        'view.hdfs.title':        'HDFS explorer',
        'view.repair.title':      'Repair',
        'view.notebooks.title':   'Notebooks',
        'view.settings.title':    'Settings',
        'dashboard.envInfo':      'Environment information',
        'dashboard.layout':       'Detected BDP layout',
        'dashboard.setup':        'Setup status',
        'dashboard.missing':      'Missing folders in the distribution:',
        'dashboard.layoutOK':     'BDP layout detected correctly. All service folders are present next to the launcher.',
        'dashboard.setupPending': 'Initial setup has not run yet. The wizard will start the next time you start services (delivered in phase F5).',
        'dashboard.setupDone':    'Initial setup complete.',
        'dashboard.namenodeMissing':'HDFS NameNode not formatted.',
        'placeholder.upcoming':   'This view will be delivered in a later phase of the plan.',
        'version.line':           'v{0} · {1}/{2}',
    },
};

let LANG = 'es';

function t(key, ...args) {
    const dict = STR[LANG] || STR.es;
    const raw  = dict[key] ?? STR.es[key] ?? key;
    return raw.replace(/\{(\d+)\}/g, (_, i) => args[Number(i)] ?? '');
}

/* ---------- View registry ------------------------------------------------- */
const VIEWS = [
    { id: 'dashboard', icon: 'layout',   render: renderDashboard },
    { id: 'services',  icon: 'server',   render: renderUpcoming  },
    { id: 'ports',     icon: 'plug',     render: renderUpcoming  },
    { id: 'consoles',  icon: 'terminal', render: renderUpcoming  },
    { id: 'hdfs',      icon: 'folder',   render: renderUpcoming  },
    { id: 'repair',    icon: 'wrench',   render: renderUpcoming  },
    { id: 'notebooks', icon: 'book',     render: renderUpcoming  },
    { id: 'settings',  icon: 'settings', render: renderUpcoming  },
];

let currentView = 'dashboard';
let envInfo = null;

/* ---------- Bootstrap ----------------------------------------------------- */
document.addEventListener('DOMContentLoaded', async () => {
    try {
        const state = await window.go.main.App.GetState();
        if (state && state.language) LANG = state.language;
    } catch (e) { /* defaults */ }

    try {
        envInfo = await window.go.main.App.GetEnvInfo();
    } catch (e) {
        envInfo = { error: String(e) };
    }

    renderBrand();
    renderNav();
    renderVersionFooter();
    renderView(currentView);
});

/* ---------- Renderers ----------------------------------------------------- */
function renderBrand() {
    document.querySelector('.c-sidebar__logo').innerHTML = iconHTML('cpu');
    document.querySelectorAll('[data-i18n]').forEach(el => {
        el.textContent = t(el.dataset.i18n);
    });
}

function renderNav() {
    const nav = document.getElementById('nav');
    nav.innerHTML = '';
    for (const v of VIEWS) {
        const btn = document.createElement('button');
        btn.className = 'c-sidebar__nav-item' + (v.id === currentView ? ' is-active' : '');
        btn.innerHTML = iconHTML(v.icon) + '<span>' + t('nav.' + v.id) + '</span>';
        btn.addEventListener('click', () => renderView(v.id));
        nav.appendChild(btn);
    }
}

function renderVersionFooter() {
    const el = document.getElementById('versionInfo');
    if (envInfo && envInfo.appVersion) {
        el.textContent = t('version.line', envInfo.appVersion, envInfo.os, envInfo.arch);
    }
}

function renderView(id) {
    currentView = id;
    const v = VIEWS.find(x => x.id === id) || VIEWS[0];
    document.getElementById('viewTitle').textContent = t('view.' + id + '.title');
    document.getElementById('viewActions').innerHTML = '';
    document.getElementById('content').innerHTML = '';
    // Update nav highlight
    document.querySelectorAll('.c-sidebar__nav-item').forEach((el, i) => {
        el.classList.toggle('is-active', VIEWS[i].id === id);
    });
    v.render(document.getElementById('content'));
}

function renderUpcoming(root) {
    root.innerHTML =
        '<div class="c-placeholder">' +
            iconHTML('info') + '<br/>' +
            '<p>' + t('placeholder.upcoming') + '</p>' +
        '</div>';
}

function renderDashboard(root) {
    if (!envInfo) {
        root.innerHTML = '<div class="c-placeholder">…</div>';
        return;
    }

    const cards = [];

    // Layout health card
    if (envInfo.distributionOK) {
        cards.push(
            '<div class="c-card">' +
                '<div class="c-card__title">' + t('dashboard.layout') + '</div>' +
                '<div class="c-card__body">' +
                    '<span class="c-badge c-badge--ok">' + iconHTML('check') + ' OK</span>' +
                    '<p style="margin: 12px 0 0; color: var(--color-text-muted);">' + t('dashboard.layoutOK') + '</p>' +
                '</div>' +
            '</div>'
        );
    } else {
        const list = (envInfo.missingDirs || []).map(x => '<li><code>' + escapeHtml(x) + '</code></li>').join('');
        cards.push(
            '<div class="c-card">' +
                '<div class="c-card__title">' + t('dashboard.layout') + '</div>' +
                '<div class="c-card__body">' +
                    '<div class="c-alert c-alert--danger">' + iconHTML('alert') +
                        '<div>' + t('dashboard.missing') + '<ul style="margin: 6px 0 0;">' + list + '</ul></div>' +
                    '</div>' +
                '</div>' +
            '</div>'
        );
    }

    // Setup status card
    const setupBody = envInfo.setupCompleted
        ? '<span class="c-badge c-badge--ok">' + iconHTML('check') + ' ' + t('dashboard.setupDone') + '</span>'
        : '<div class="c-alert c-alert--warn">' + iconHTML('alert') + '<div>' + t('dashboard.setupPending') +
            (!envInfo.namenodeFormatted ? '<br/><small>' + t('dashboard.namenodeMissing') + '</small>' : '') +
          '</div></div>';
    cards.push(
        '<div class="c-card">' +
            '<div class="c-card__title">' + t('dashboard.setup') + '</div>' +
            '<div class="c-card__body">' + setupBody + '</div>' +
        '</div>'
    );

    // Env info card
    const kv = [];
    kv.push(['OS', envInfo.os + ' / ' + envInfo.arch]);
    kv.push(['Script dir', envInfo.scriptDir]);
    for (const [k, v] of Object.entries(envInfo.servicePaths || {})) {
        kv.push([k, v]);
    }
    const kvHTML = kv.map(([k, v]) =>
        '<div class="c-kv__k">' + escapeHtml(k) + '</div><div class="c-kv__v">' + escapeHtml(v) + '</div>'
    ).join('');
    cards.push(
        '<div class="c-card" style="grid-column: 1 / -1;">' +
            '<div class="c-card__title">' + t('dashboard.envInfo') + '</div>' +
            '<div class="c-card__body"><div class="c-kv">' + kvHTML + '</div></div>' +
        '</div>'
    );

    root.innerHTML = '<div class="c-card-grid">' + cards.join('') + '</div>';
}

function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({
        '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'
    }[c]));
}
