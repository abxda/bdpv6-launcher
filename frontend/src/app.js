/* =========================================================================
 *  BDPV6 Launcher — frontend bootstrap
 *
 *  Vanilla ES2020. The Wails runtime injects:
 *    - window.go.main.App.<Method>(...)      → bound Go methods
 *    - window.runtime.EventsOn(name, cb)     → live events from Go
 *    - window.runtime.BrowserOpenURL(url)    → open URL in system browser
 *
 *  Layout: app shell (sidebar + main area) with a small client-side router.
 *  Each tab has its own render function. Live state (statuses, logs) is held
 *  in module-level objects refreshed by Wails events.
 * ========================================================================= */

/* ---------- ICONS (Lucide, inlined as SVG) -------------------------------- */
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
    play:       '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="6 3 20 12 6 21 6 3"/></svg>',
    stop:       '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="5" y="5" rx="1"/></svg>',
    refresh:    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/></svg>',
    trash:      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',
    copy:       '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="8" y="8" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>',
};

function iconHTML(name) { return ICONS[name] || ICONS.info; }

/* ---------- i18n ---------------------------------------------------------- */
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
        'dashboard.services':     'Servicios',
        'services.startAll':      'Iniciar TODO',
        'services.stopAll':       'Detener TODO',
        'services.start':         'Iniciar',
        'services.stop':          'Detener',
        'services.logs':          'Consola',
        'services.status.running':'En ejecución',
        'services.status.healthy':'Sano',
        'services.status.degraded':'Degradado',
        'services.status.stopped':'Detenido',
        'services.col.name':      'Servicio',
        'services.col.status':    'Estado',
        'services.col.port':      'Puerto',
        'services.col.pid':       'PID',
        'services.col.actions':   'Acciones',
        'services.col.detail':    'Detalle',
        'consoles.noService':     'Selecciona un servicio para ver su consola.',
        'consoles.empty':         'Sin salida todavía. Inicia el servicio para ver el log en vivo.',
        'consoles.clear':         'Limpiar',
        'consoles.copy':          'Copiar',
        'consoles.autoscroll':    'Autoscroll',
        'consoles.filter.all':    'Todo',
        'consoles.filter.error':  'Solo errores',
        'consoles.filter.warn':   'Warn+errores',
        'consoles.search':        'Filtrar líneas…',
        'ports.col.service':      'Servicio',
        'ports.col.desired':      'Puerto',
        'ports.col.status':       'Estado',
        'ports.col.owner':        'Ocupado por',
        'ports.col.actions':      'Acciones',
        'ports.state.free':       'Libre',
        'ports.state.ours':       'Nuestro',
        'ports.state.other':      'Ocupado',
        'ports.refresh':          'Refrescar',
        'ports.suggest':          'Sugerir libre',
        'ports.edit':             'Cambiar puerto',
        'ports.editPrompt':       'Nuevo puerto para {0} (1024-65534):',
        'ports.editApplied':      'Puerto guardado. Reinicia el servicio para aplicar.',
        'ports.suggestApplied':   'Sugerido: {0}. Guardado como override.',
        'ports.noFree':           'No se encontró puerto libre cercano.',
        'ports.help':             'Las filas en rojo están ocupadas por otro proceso. Usa "Sugerir libre" para encontrar un puerto alternativo o cierra el proceso conflictivo.',
        'view.wizard.title':      'Configuración inicial',
        'wizard.intro':           'Este asistente prepara el entorno BDP la primera vez que lo usas. Solo necesitas ejecutarlo una vez por distribución.',
        'wizard.runAll':          'Ejecutar todo',
        'wizard.runStep':         'Ejecutar',
        'wizard.backToDashboard': 'Volver al Dashboard',
        'wizard.startSetup':      'Iniciar configuración',
        'wizard.status.pending':  'Pendiente',
        'wizard.status.running':  'Ejecutando…',
        'wizard.status.done':     'Completado',
        'wizard.status.failed':   'Falló',
        'repair.intro':           'Acciones destructivas para recuperar un entorno corrupto. Cada acción pide confirmación explícita antes de ejecutarse.',
        'repair.run':             'Ejecutar',
        'repair.confirmKeyword':  'BORRAR',
        'repair.confirmPrompt':   'Esta acción es destructiva.\\n\\nPara confirmar, escribe la palabra "{0}" (mayúsculas):',
        'repair.cancelled':       'Acción cancelada.',
        'repair.jupyter.title':   'Reparar Jupyter',
        'repair.jupyter.desc':    'Desinstala y reinstala jupyter_core, jupyter_client, jupyter_server, notebook y jupyterlab usando el Python embebido. Útil si Jupyter no arranca o crashea al abrir un notebook. Requiere conexión a internet.',
        'repair.clean.title':     'Limpiar datos y logs',
        'repair.clean.desc':      'Borra completamente data/ y logs/ (incluyendo HDFS y Kafka). También limpia __pycache__ y resetea el estado de configuración. Equivale a partir de cero. NECESITARÁS RE-EJECUTAR EL ASISTENTE después.',
        'repair.hdfs.title':      'Reformatear HDFS',
        'repair.hdfs.desc':       'Re-ejecuta hdfs namenode -format. Pierdes todos los archivos en HDFS pero conservas Jupyter, Elasticsearch y Kafka. Útil si HDFS no arranca por estado sucio.',
        'repair.kafka.title':     'Reformatear Kafka',
        'repair.kafka.desc':      'Genera un nuevo cluster id y reformatea el log de KRaft. Pierdes todos los topics. Útil si Kafka no arranca por estado inconsistente.',
        'hdfs.root':              '/ (raíz HDFS)',
        'hdfs.refresh':           'Refrescar',
        'hdfs.error':             'No se pudo conectar al NameNode. ¿HDFS está iniciado?',
        'hdfs.empty':             '(directorio vacío)',
        'notebooks.empty':        'No hay notebooks en la carpeta. Coloca archivos .ipynb dentro de notebooks/.',
        'notebooks.open':         'Abrir',
        'notebooks.needsJupyter': 'Jupyter no está corriendo. Inícialo desde la pestaña Servicios.',
        'notebooks.col.name':     'Archivo',
        'notebooks.col.size':     'Tamaño',
        'notebooks.col.mod':      'Modificación',
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
        'dashboard.services':     'Services',
        'services.startAll':      'Start ALL',
        'services.stopAll':       'Stop ALL',
        'services.start':         'Start',
        'services.stop':          'Stop',
        'services.logs':          'Console',
        'services.status.running':'Running',
        'services.status.healthy':'Healthy',
        'services.status.degraded':'Degraded',
        'services.status.stopped':'Stopped',
        'services.col.name':      'Service',
        'services.col.status':    'Status',
        'services.col.port':      'Port',
        'services.col.pid':       'PID',
        'services.col.actions':   'Actions',
        'services.col.detail':    'Detail',
        'consoles.noService':     'Select a service to view its console.',
        'consoles.empty':         'No output yet. Start the service to see live logs.',
        'consoles.clear':         'Clear',
        'consoles.copy':          'Copy',
        'consoles.autoscroll':    'Autoscroll',
        'consoles.filter.all':    'All',
        'consoles.filter.error':  'Errors only',
        'consoles.filter.warn':   'Warn + errors',
        'consoles.search':        'Filter lines…',
        'ports.col.service':      'Service',
        'ports.col.desired':      'Port',
        'ports.col.status':       'Status',
        'ports.col.owner':        'Owned by',
        'ports.col.actions':      'Actions',
        'ports.state.free':       'Free',
        'ports.state.ours':       'Ours',
        'ports.state.other':      'In use',
        'ports.refresh':          'Refresh',
        'ports.suggest':          'Suggest free',
        'ports.edit':             'Change port',
        'ports.editPrompt':       'New port for {0} (1024-65534):',
        'ports.editApplied':      'Port saved. Restart the service to apply.',
        'ports.suggestApplied':   'Suggested: {0}. Saved as override.',
        'ports.noFree':           'No free port found nearby.',
        'ports.help':             'Rows in red are bound by another process. Use "Suggest free" to find an alternative, or close the conflicting process.',
        'view.wizard.title':      'First-run setup',
        'wizard.intro':           'This wizard prepares the BDP environment the first time you use it. You only need to run it once per distribution.',
        'wizard.runAll':          'Run all',
        'wizard.runStep':         'Run',
        'wizard.backToDashboard': 'Back to Dashboard',
        'wizard.startSetup':      'Start setup',
        'wizard.status.pending':  'Pending',
        'wizard.status.running':  'Running…',
        'wizard.status.done':     'Done',
        'wizard.status.failed':   'Failed',
        'repair.intro':           'Destructive actions to recover a corrupted environment. Each action asks for explicit confirmation before running.',
        'repair.run':             'Run',
        'repair.confirmKeyword':  'DELETE',
        'repair.confirmPrompt':   'This action is destructive.\\n\\nTo confirm, type the word "{0}" (uppercase):',
        'repair.cancelled':       'Action cancelled.',
        'repair.jupyter.title':   'Repair Jupyter',
        'repair.jupyter.desc':    'Uninstall and reinstall jupyter_core, jupyter_client, jupyter_server, notebook and jupyterlab using the embedded Python. Useful when Jupyter will not start or crashes on notebook open. Requires internet.',
        'repair.clean.title':     'Clean data and logs',
        'repair.clean.desc':      'Wipes data/ and logs/ entirely (HDFS and Kafka included). Also clears __pycache__ and resets the setup state. Equivalent to starting from scratch. YOU WILL NEED TO RE-RUN THE WIZARD afterwards.',
        'repair.hdfs.title':      'Reformat HDFS',
        'repair.hdfs.desc':       'Re-runs hdfs namenode -format. You lose every file in HDFS but Jupyter, Elasticsearch and Kafka are preserved. Useful when HDFS will not start due to dirty state.',
        'repair.kafka.title':     'Reformat Kafka',
        'repair.kafka.desc':      'Generates a new cluster id and reformats the KRaft log. You lose every topic. Useful when Kafka will not start due to inconsistent state.',
        'hdfs.root':              '/ (HDFS root)',
        'hdfs.refresh':           'Refresh',
        'hdfs.error':             'Could not reach NameNode. Is HDFS running?',
        'hdfs.empty':             '(empty directory)',
        'notebooks.empty':        'No notebooks in the folder. Drop .ipynb files inside notebooks/.',
        'notebooks.open':         'Open',
        'notebooks.needsJupyter': 'Jupyter is not running. Start it from the Services tab.',
        'notebooks.col.name':     'File',
        'notebooks.col.size':     'Size',
        'notebooks.col.mod':      'Modified',
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

/* ---------- Live state ---------------------------------------------------- */
const STATE = {
    envInfo: null,
    services: [],            // [{id, name, port}]
    statuses: {},            // id → Status
    logs: {},                // id → [Line]
    activeConsole: null,     // id of the currently viewed service in Consoles tab
};

/* ---------- View registry ------------------------------------------------- */
const VIEWS = [
    { id: 'dashboard', icon: 'layout',   render: renderDashboard, onLeave: null },
    { id: 'services',  icon: 'server',   render: renderServices,  onLeave: null },
    { id: 'ports',     icon: 'plug',     render: renderPorts,     onLeave: stopPortsRefresh },
    { id: 'consoles',  icon: 'terminal', render: renderConsoles,  onLeave: null },
    { id: 'hdfs',      icon: 'folder',   render: renderHDFS,      onLeave: null },
    { id: 'repair',    icon: 'wrench',   render: renderRepair,    onLeave: null },
    { id: 'notebooks', icon: 'book',     render: renderNotebooks, onLeave: null },
    { id: 'settings',  icon: 'settings', render: renderUpcoming,  onLeave: null },
    // Hidden — reachable from the dashboard alert when setup is needed.
    { id: 'wizard',    icon: 'wrench',   render: renderWizard,    onLeave: null, hidden: true },
];

let currentView = 'dashboard';

/* ---------- Bootstrap ----------------------------------------------------- */
document.addEventListener('DOMContentLoaded', async () => {
    try {
        const s = await window.go.main.App.GetState();
        if (s && s.language) LANG = s.language;
    } catch (e) { /* defaults */ }

    try { STATE.envInfo  = await window.go.main.App.GetEnvInfo(); } catch (e) { STATE.envInfo  = { error: String(e) }; }
    try { STATE.services = await window.go.main.App.ListServices() || []; } catch (e) { STATE.services = []; }
    try {
        const arr = await window.go.main.App.GetStatuses();
        for (const st of (arr || [])) STATE.statuses[st.id] = st;
    } catch (e) {}

    subscribeRuntimeEvents();
    renderBrand();
    renderNav();
    renderVersionFooter();
    renderView(currentView);
});

function subscribeRuntimeEvents() {
    if (!window.runtime || !window.runtime.EventsOn) return;

    window.runtime.EventsOn('status:tick', (arr) => {
        for (const st of (arr || [])) STATE.statuses[st.id] = st;
        if (currentView === 'services')  renderView('services');
        else if (currentView === 'dashboard') renderDashboard(document.getElementById('content'));
    });

    // One subscription per service for live log streaming.
    for (const svc of STATE.services) {
        const eventName = 'service:' + svc.id + ':log';
        window.runtime.EventsOn(eventName, (line) => {
            if (!STATE.logs[svc.id]) STATE.logs[svc.id] = [];
            STATE.logs[svc.id].push(line);
            // Cap the in-memory frontend buffer to 2000 lines — backend
            // already does this but we don't want runaway growth either.
            if (STATE.logs[svc.id].length > 2000) {
                STATE.logs[svc.id].splice(0, STATE.logs[svc.id].length - 2000);
            }
            if (currentView === 'consoles' && STATE.activeConsole === svc.id) {
                appendConsoleLine(line);
            }
        });
    }
}

/* ---------- Sidebar / nav ------------------------------------------------- */
function renderBrand() {
    document.querySelector('.c-sidebar__logo').innerHTML = iconHTML('cpu');
    document.querySelectorAll('[data-i18n]').forEach(el => { el.textContent = t(el.dataset.i18n); });
}

function renderNav() {
    const nav = document.getElementById('nav');
    nav.innerHTML = '';
    for (const v of VIEWS) {
        if (v.hidden) continue;
        const btn = document.createElement('button');
        btn.className = 'c-sidebar__nav-item' + (v.id === currentView ? ' is-active' : '');
        btn.innerHTML = iconHTML(v.icon) + '<span>' + t('nav.' + v.id) + '</span>';
        btn.addEventListener('click', () => renderView(v.id));
        nav.appendChild(btn);
    }
}

function renderVersionFooter() {
    const el = document.getElementById('versionInfo');
    if (STATE.envInfo && STATE.envInfo.appVersion) {
        el.textContent = t('version.line', STATE.envInfo.appVersion, STATE.envInfo.os, STATE.envInfo.arch);
    }
}

function renderView(id) {
    const prev = VIEWS.find(x => x.id === currentView);
    if (prev && prev.onLeave) prev.onLeave();
    currentView = id;
    const v = VIEWS.find(x => x.id === id) || VIEWS[0];
    document.getElementById('viewTitle').textContent = t('view.' + id + '.title');
    document.getElementById('viewActions').innerHTML = '';
    document.getElementById('content').innerHTML = '';
    // Update only visible nav items.
    const visibleViews = VIEWS.filter(v => !v.hidden);
    document.querySelectorAll('.c-sidebar__nav-item').forEach((el, i) => {
        el.classList.toggle('is-active', visibleViews[i] && visibleViews[i].id === id);
    });
    v.render(document.getElementById('content'));
}

function renderUpcoming(root) {
    root.innerHTML =
        '<div class="c-placeholder">' + iconHTML('info') + '<br/><p>' + t('placeholder.upcoming') + '</p></div>';
}

/* ---------- Dashboard ----------------------------------------------------- */
function renderDashboard(root) {
    if (!STATE.envInfo) { root.innerHTML = '<div class="c-placeholder">…</div>'; return; }
    const env = STATE.envInfo;

    const cards = [];
    if (env.distributionOK) {
        cards.push(card(t('dashboard.layout'),
            '<span class="c-badge c-badge--ok">' + iconHTML('check') + ' OK</span>' +
            '<p style="margin:12px 0 0;color:var(--color-text-muted);">' + t('dashboard.layoutOK') + '</p>'));
    } else {
        const list = (env.missingDirs || []).map(x => '<li><code>' + esc(x) + '</code></li>').join('');
        cards.push(card(t('dashboard.layout'),
            '<div class="c-alert c-alert--danger">' + iconHTML('alert') +
            '<div>' + t('dashboard.missing') + '<ul style="margin:6px 0 0;">' + list + '</ul></div></div>'));
    }

    const setupBody = env.setupCompleted
        ? '<span class="c-badge c-badge--ok">' + iconHTML('check') + ' ' + t('dashboard.setupDone') + '</span>'
        : '<div class="c-alert c-alert--warn">' + iconHTML('alert') + '<div>' + t('dashboard.setupPending') +
          (!env.namenodeFormatted ? '<br/><small>' + t('dashboard.namenodeMissing') + '</small>' : '') +
          '<br/><br/><button class="c-btn c-btn--primary" id="actStartWizard">' + iconHTML('wrench') + ' ' + t('wizard.startSetup') + '</button>' +
          '</div></div>';
    cards.push(card(t('dashboard.setup'), setupBody));

    // Compact services summary
    const rows = STATE.services.map(svc => {
        const st = STATE.statuses[svc.id] || {};
        return '<tr>' +
            '<td>' + esc(svc.name) + '</td>' +
            '<td>' + statusBadge(st) + '</td>' +
            '<td><code>' + svc.port + '</code></td>' +
            '<td><code>' + (st.pid || '—') + '</code></td>' +
            '</tr>';
    }).join('');
    const svcTable = '<table class="c-table"><thead><tr>' +
        '<th>' + t('services.col.name')   + '</th>' +
        '<th>' + t('services.col.status') + '</th>' +
        '<th>' + t('services.col.port')   + '</th>' +
        '<th>' + t('services.col.pid')    + '</th>' +
        '</tr></thead><tbody>' + rows + '</tbody></table>';
    cards.push(card(t('dashboard.services'), svcTable, 'grid-column:1 / -1;'));

    // Env / paths
    const kv = [];
    kv.push(['OS', env.os + ' / ' + env.arch]);
    kv.push(['Script dir', env.scriptDir]);
    for (const [k, v] of Object.entries(env.servicePaths || {})) kv.push([k, v]);
    const kvHTML = kv.map(([k, v]) =>
        '<div class="c-kv__k">' + esc(k) + '</div><div class="c-kv__v">' + esc(v) + '</div>'
    ).join('');
    cards.push(card(t('dashboard.envInfo'),
        '<div class="c-kv">' + kvHTML + '</div>', 'grid-column:1 / -1;'));

    root.innerHTML = '<div class="c-card-grid">' + cards.join('') + '</div>';

    const wizBtn = document.getElementById('actStartWizard');
    if (wizBtn) wizBtn.addEventListener('click', () => renderView('wizard'));
}

/* ---------- Wizard (hidden view, reached from Dashboard) ------------------ */
const SETUP_LOG_KEY = '__setup__';

async function renderWizard(root) {
    document.getElementById('viewActions').innerHTML =
        '<button class="c-btn" id="actWizardBack">' + iconHTML('layout') + ' ' + t('wizard.backToDashboard') + '</button>' +
        '<button class="c-btn c-btn--primary" id="actWizardRunAll">' + iconHTML('play') + ' ' + t('wizard.runAll') + '</button>';
    document.getElementById('actWizardBack').addEventListener('click', () => renderView('dashboard'));
    document.getElementById('actWizardRunAll').addEventListener('click', wizardRunAll);

    // Make sure we have logs and a setup:log subscription.
    if (!STATE.logs[SETUP_LOG_KEY]) {
        try { STATE.logs[SETUP_LOG_KEY] = await window.go.main.App.GetSetupLogs() || []; }
        catch (e) { STATE.logs[SETUP_LOG_KEY] = []; }
    }
    if (!STATE._setupSubscribed && window.runtime && window.runtime.EventsOn) {
        window.runtime.EventsOn('service:setup:log', (line) => {
            if (!STATE.logs[SETUP_LOG_KEY]) STATE.logs[SETUP_LOG_KEY] = [];
            STATE.logs[SETUP_LOG_KEY].push(line);
            if (currentView === 'wizard') appendWizardLine(line);
        });
        STATE._setupSubscribed = true;
    }

    await paintWizard(root);
}

async function paintWizard(root) {
    let steps = [];
    try { steps = await window.go.main.App.GetSetupSteps() || []; } catch (e) { steps = []; }

    const stepCards = steps.map(s => {
        const badge = (
            s.status === 'done'    ? '<span class="c-badge c-badge--ok">'    + iconHTML('check') + ' ' + t('wizard.status.done')    + '</span>' :
            s.status === 'running' ? '<span class="c-badge c-badge--info">'  + t('wizard.status.running') + '</span>' :
            s.status === 'failed'  ? '<span class="c-badge c-badge--danger">' + iconHTML('alert') + ' ' + t('wizard.status.failed') + '</span>' :
                                     '<span class="c-badge c-badge--muted">'  + t('wizard.status.pending') + '</span>'
        );
        const err = s.errorMsg ? '<small style="color:var(--color-danger); display:block; margin-top:6px;">' + esc(s.errorMsg) + '</small>' : '';
        return '<div class="c-card">' +
            '<div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px;">' +
                '<div><strong>' + esc(s.name) + '</strong> ' + badge + '</div>' +
                '<button class="c-btn c-btn--primary" data-step="' + s.id + '">' + iconHTML('play') + ' ' + t('wizard.runStep') + '</button>' +
            '</div>' +
            '<small style="color:var(--color-text-muted);">' + esc(s.description) + '</small>' +
            err +
        '</div>';
    }).join('');

    root.innerHTML =
        '<div class="c-alert c-alert--info">' + iconHTML('info') + '<div>' + t('wizard.intro') + '</div></div>' +
        '<div class="c-card-grid" style="margin-bottom: var(--s-5);">' + stepCards + '</div>' +
        '<div class="c-console" style="height: 320px;">' +
            '<div class="c-console__toolbar"><strong style="color:var(--color-text-muted);">Salida del asistente</strong></div>' +
            '<pre class="c-console__pane" id="wizardPane"></pre>' +
        '</div>';

    root.querySelectorAll('button[data-step]').forEach(btn => {
        btn.addEventListener('click', async () => {
            try { await window.go.main.App.RunSetupStep(btn.dataset.step); }
            catch (e) { /* surfaced via step.errorMsg on repaint */ }
            paintWizard(root);
        });
    });

    paintWizardLog();
}

function paintWizardLog() {
    const pane = document.getElementById('wizardPane');
    if (!pane) return;
    const lines = STATE.logs[SETUP_LOG_KEY] || [];
    if (!lines.length) { pane.innerHTML = '<span style="color:var(--color-text-muted);">(sin salida aún)</span>'; return; }
    pane.innerHTML = lines.map(formatLineHTML).join('\n');
    pane.scrollTop = pane.scrollHeight;
}

function appendWizardLine(line) {
    const pane = document.getElementById('wizardPane');
    if (!pane) return;
    if (pane.children.length === 1 && !pane.children[0].classList) pane.innerHTML = '';
    pane.insertAdjacentHTML('beforeend', formatLineHTML(line) + '\n');
    pane.scrollTop = pane.scrollHeight;
}

async function wizardRunAll() {
    try { await window.go.main.App.RunSetupAll(); }
    catch (e) { /* surfaced per-step */ }
    paintWizard(document.getElementById('content'));
}

/* ---------- HDFS tab ------------------------------------------------------ */
async function renderHDFS(root) {
    document.getElementById('viewActions').innerHTML =
        '<button class="c-btn" id="actHdfsRefresh">' + iconHTML('refresh') + ' ' + t('hdfs.refresh') + '</button>';
    document.getElementById('actHdfsRefresh').addEventListener('click', () => renderHDFS(root));

    root.innerHTML = '<div class="c-hdfs-tree" id="hdfsTree"><div class="c-placeholder">…</div></div>';
    const treeRoot = document.getElementById('hdfsTree');
    treeRoot.innerHTML = '';

    const rootNode = await hdfsBuildNode('/', t('hdfs.root'), true);
    treeRoot.appendChild(rootNode);
}

async function hdfsBuildNode(path, label, expandedInitially) {
    const node = document.createElement('div');
    node.className = 'c-hdfs-node';
    const isDir = true;
    node.innerHTML =
        '<div class="c-hdfs-node__row" data-path="' + esc(path) + '">' +
            '<span class="c-hdfs-node__chev">▶</span> ' +
            iconHTML('folder') + ' <strong>' + esc(label) + '</strong>' +
        '</div>' +
        '<div class="c-hdfs-node__children" style="display:none;"></div>';
    const row = node.querySelector('.c-hdfs-node__row');
    const childrenEl = node.querySelector('.c-hdfs-node__children');
    let loaded = false;

    async function toggle() {
        if (!loaded) {
            try {
                const entries = await window.go.main.App.ListHDFS(path);
                if (!entries || entries.length === 0) {
                    childrenEl.innerHTML = '<div class="c-hdfs-empty">' + t('hdfs.empty') + '</div>';
                } else {
                    childrenEl.innerHTML = '';
                    entries.sort((a, b) => (a.type === b.type) ? a.name.localeCompare(b.name) : (a.type === 'DIRECTORY' ? -1 : 1));
                    for (const entry of entries) {
                        if (entry.type === 'DIRECTORY') {
                            const child = await hdfsBuildNode(joinPath(path, entry.name), entry.name, false);
                            childrenEl.appendChild(child);
                        } else {
                            const f = document.createElement('div');
                            f.className = 'c-hdfs-file';
                            f.innerHTML = iconHTML('book') + ' ' + esc(entry.name) +
                                ' <small style="color:var(--color-text-muted);">' + humanSize(entry.length) + '</small>';
                            childrenEl.appendChild(f);
                        }
                    }
                }
            } catch (e) {
                childrenEl.innerHTML = '<div class="c-alert c-alert--danger">' + iconHTML('alert') + '<div>' + t('hdfs.error') + '<br/><small>' + esc(String(e)) + '</small></div></div>';
            }
            loaded = true;
        }
        const expanded = childrenEl.style.display !== 'none';
        childrenEl.style.display = expanded ? 'none' : 'block';
        row.querySelector('.c-hdfs-node__chev').textContent = expanded ? '▶' : '▼';
    }

    row.addEventListener('click', toggle);
    if (expandedInitially) await toggle();
    return node;
}

function joinPath(a, b) {
    if (a === '/' || a === '') return '/' + b;
    return a.replace(/\/$/, '') + '/' + b;
}

function humanSize(bytes) {
    if (!bytes) return '0';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0; let n = Number(bytes);
    while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
    return n.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

/* ---------- Notebooks tab ------------------------------------------------- */
async function renderNotebooks(root) {
    document.getElementById('viewActions').innerHTML = '';

    let files = [];
    try { files = await window.go.main.App.ListNotebooks() || []; } catch (e) {}

    const jurl = await (window.go.main.App.JupyterURL ? window.go.main.App.JupyterURL() : Promise.resolve(''));
    const jupyterReady = !!jurl;
    const warning = jupyterReady ? '' :
        '<div class="c-alert c-alert--warn">' + iconHTML('alert') + '<div>' + t('notebooks.needsJupyter') + '</div></div>';

    if (!files.length) {
        root.innerHTML = warning + '<div class="c-placeholder">' + t('notebooks.empty') + '</div>';
        return;
    }

    const rows = files.map(f => {
        const date = new Date(f.modTime).toLocaleString();
        const size = f.isDir ? '—' : humanSize(f.size);
        const icon = f.isDir ? 'folder' : 'book';
        const openBtn = (!f.isDir && jupyterReady) ?
            '<button class="c-btn c-btn--primary" data-open="' + esc(f.name) + '">' + iconHTML('book') + ' ' + t('notebooks.open') + '</button>' : '';
        return '<tr>' +
            '<td>' + iconHTML(icon) + ' ' + esc(f.name) + '</td>' +
            '<td>' + size + '</td>' +
            '<td>' + esc(date) + '</td>' +
            '<td class="c-table__actions">' + openBtn + '</td>' +
        '</tr>';
    }).join('');

    root.innerHTML = warning +
        '<table class="c-table"><thead><tr>' +
            '<th>' + t('notebooks.col.name') + '</th>' +
            '<th>' + t('notebooks.col.size') + '</th>' +
            '<th>' + t('notebooks.col.mod')  + '</th>' +
            '<th></th>' +
        '</tr></thead><tbody>' + rows + '</tbody></table>';

    root.querySelectorAll('button[data-open]').forEach(btn => {
        btn.addEventListener('click', async () => {
            try { await window.go.main.App.OpenNotebook(btn.dataset.open); }
            catch (e) { alert('Error: ' + e); }
        });
    });
}

/* ---------- Repair tab ---------------------------------------------------- */
const REPAIR_LOG_KEY = '__repair__';
const REPAIR_ACTIONS = [
    { id: 'repair_jupyter', titleKey: 'repair.jupyter.title', descKey: 'repair.jupyter.desc', destructive: false },
    { id: 'reformat_hdfs',  titleKey: 'repair.hdfs.title',    descKey: 'repair.hdfs.desc',    destructive: true  },
    { id: 'reformat_kafka', titleKey: 'repair.kafka.title',   descKey: 'repair.kafka.desc',   destructive: true  },
    { id: 'clean',          titleKey: 'repair.clean.title',   descKey: 'repair.clean.desc',   destructive: true  },
];

async function renderRepair(root) {
    document.getElementById('viewActions').innerHTML = '';

    if (!STATE.logs[REPAIR_LOG_KEY]) {
        try { STATE.logs[REPAIR_LOG_KEY] = await window.go.main.App.GetRepairLogs() || []; }
        catch (e) { STATE.logs[REPAIR_LOG_KEY] = []; }
    }
    if (!STATE._repairSubscribed && window.runtime && window.runtime.EventsOn) {
        window.runtime.EventsOn('service:repair:log', (line) => {
            if (!STATE.logs[REPAIR_LOG_KEY]) STATE.logs[REPAIR_LOG_KEY] = [];
            STATE.logs[REPAIR_LOG_KEY].push(line);
            if (currentView === 'repair') appendRepairLine(line);
        });
        STATE._repairSubscribed = true;
    }

    const cards = REPAIR_ACTIONS.map(a => {
        const danger = a.destructive ? 'c-card c-card--danger-outline' : 'c-card';
        return '<div class="' + danger + '">' +
            '<div style="display:flex; justify-content:space-between; align-items:flex-start; gap: 12px;">' +
                '<div>' +
                    '<strong>' + esc(t(a.titleKey)) + '</strong>' +
                    '<p style="margin: 6px 0 0; color:var(--color-text-muted); font-size: 13px;">' + esc(t(a.descKey)) + '</p>' +
                '</div>' +
                '<button class="c-btn ' + (a.destructive ? 'c-btn--danger' : 'c-btn--primary') + '" data-action="' + a.id + '" data-destructive="' + a.destructive + '">' +
                    iconHTML('wrench') + ' ' + t('repair.run') +
                '</button>' +
            '</div>' +
        '</div>';
    }).join('');

    root.innerHTML =
        '<div class="c-alert c-alert--warn">' + iconHTML('alert') + '<div>' + t('repair.intro') + '</div></div>' +
        '<div class="c-card-grid" style="margin-bottom: var(--s-5);">' + cards + '</div>' +
        '<div class="c-console" style="height: 320px;">' +
            '<div class="c-console__toolbar"><strong style="color:var(--color-text-muted);">Salida de las acciones de reparación</strong></div>' +
            '<pre class="c-console__pane" id="repairPane"></pre>' +
        '</div>';

    root.querySelectorAll('button[data-action]').forEach(btn => {
        btn.addEventListener('click', () => runRepairAction(btn.dataset.action, btn.dataset.destructive === 'true'));
    });

    paintRepairLog();
}

async function runRepairAction(action, destructive) {
    if (destructive) {
        const kw = t('repair.confirmKeyword');
        const v = prompt(t('repair.confirmPrompt', kw));
        if (v !== kw) {
            alert(t('repair.cancelled'));
            return;
        }
    } else {
        if (!confirm(t('repair.run') + ' "' + action + '"?')) return;
    }
    try { await window.go.main.App.RunRepair(action); }
    catch (e) { alert('Error: ' + e); }
}

function paintRepairLog() {
    const pane = document.getElementById('repairPane');
    if (!pane) return;
    const lines = STATE.logs[REPAIR_LOG_KEY] || [];
    if (!lines.length) { pane.innerHTML = '<span style="color:var(--color-text-muted);">(sin salida aún)</span>'; return; }
    pane.innerHTML = lines.map(formatLineHTML).join('\n');
    pane.scrollTop = pane.scrollHeight;
}
function appendRepairLine(line) {
    const pane = document.getElementById('repairPane');
    if (!pane) return;
    if (pane.children.length === 1 && !pane.children[0].classList) pane.innerHTML = '';
    pane.insertAdjacentHTML('beforeend', formatLineHTML(line) + '\n');
    pane.scrollTop = pane.scrollHeight;
}

/* ---------- Services tab -------------------------------------------------- */
function renderServices(root) {
    // Header actions
    document.getElementById('viewActions').innerHTML =
        '<button class="c-btn c-btn--primary" id="actStartAll">' + iconHTML('play') + ' ' + t('services.startAll') + '</button>' +
        '<button class="c-btn c-btn--danger"  id="actStopAll">'  + iconHTML('stop') + ' ' + t('services.stopAll')  + '</button>';
    document.getElementById('actStartAll').addEventListener('click', startAll);
    document.getElementById('actStopAll').addEventListener('click', stopAll);

    if (!STATE.services.length) {
        root.innerHTML = '<div class="c-placeholder">' + iconHTML('info') + '<br/>' + t('placeholder.upcoming') + '</div>';
        return;
    }

    const rows = STATE.services.map(svc => {
        const st = STATE.statuses[svc.id] || {};
        return '<tr>' +
            '<td><strong>' + esc(svc.name) + '</strong><br/><small style="color:var(--color-text-muted);">' + esc(svc.id) + '</small></td>' +
            '<td>' + statusBadge(st) + '</td>' +
            '<td><code>' + svc.port + '</code></td>' +
            '<td><code>' + (st.pid || '—') + '</code></td>' +
            '<td><small>' + esc(st.detail || '') + '</small></td>' +
            '<td class="c-table__actions">' +
                '<button class="c-btn c-btn--primary" data-act="start" data-id="' + svc.id + '">' + iconHTML('play') + '</button>' +
                '<button class="c-btn c-btn--danger"  data-act="stop"  data-id="' + svc.id + '">' + iconHTML('stop') + '</button>' +
                '<button class="c-btn"                data-act="logs"  data-id="' + svc.id + '">' + iconHTML('terminal') + '</button>' +
            '</td>' +
        '</tr>';
    }).join('');

    root.innerHTML =
        '<table class="c-table"><thead><tr>' +
            '<th>' + t('services.col.name')    + '</th>' +
            '<th>' + t('services.col.status')  + '</th>' +
            '<th>' + t('services.col.port')    + '</th>' +
            '<th>' + t('services.col.pid')     + '</th>' +
            '<th>' + t('services.col.detail')  + '</th>' +
            '<th>' + t('services.col.actions') + '</th>' +
        '</tr></thead><tbody>' + rows + '</tbody></table>';

    root.querySelectorAll('button[data-act]').forEach(btn => {
        btn.addEventListener('click', () => {
            const act = btn.dataset.act; const id = btn.dataset.id;
            if (act === 'start') startService(id);
            else if (act === 'stop') stopService(id);
            else if (act === 'logs') { STATE.activeConsole = id; renderView('consoles'); }
        });
    });
}

async function startService(id) {
    try { await window.go.main.App.StartService(id); } catch (e) { alert('Error: ' + e); }
}
async function stopService(id) {
    try { await window.go.main.App.StopService(id); } catch (e) { alert('Error: ' + e); }
}
async function startAll() { for (const s of STATE.services) await startService(s.id); }
async function stopAll()  { for (const s of [...STATE.services].reverse()) await stopService(s.id); }

/* ---------- Ports tab ----------------------------------------------------- */
let portsRefreshTimer = null;

function stopPortsRefresh() {
    if (portsRefreshTimer) { clearInterval(portsRefreshTimer); portsRefreshTimer = null; }
}

async function renderPorts(root) {
    document.getElementById('viewActions').innerHTML =
        '<button class="c-btn" id="actPortsRefresh">' + iconHTML('refresh') + ' ' + t('ports.refresh') + '</button>';
    document.getElementById('actPortsRefresh').addEventListener('click', () => paintPorts(root));

    await paintPorts(root);

    // Auto-refresh while this view is active.
    stopPortsRefresh();
    portsRefreshTimer = setInterval(() => { if (currentView === 'ports') paintPorts(root); }, 5000);
}

async function paintPorts(root) {
    let probes = [];
    try { probes = await window.go.main.App.ScanPorts() || []; } catch (e) { probes = []; }

    const help = '<div class="c-alert c-alert--info">' + iconHTML('info') + '<div>' + t('ports.help') + '</div></div>';

    const rows = probes.map(p => {
        const rowClass = p.status === 'other' ? 'c-table__row--danger' : (p.status === 'ours' ? '' : '');
        const badge =
            p.status === 'free'  ? '<span class="c-badge c-badge--muted">' + t('ports.state.free')  + '</span>' :
            p.status === 'ours'  ? '<span class="c-badge c-badge--ok">'    + t('ports.state.ours')  + '</span>' :
                                   '<span class="c-badge c-badge--danger">' + t('ports.state.other') + '</span>';
        const owner = (p.status === 'other' && p.owner) ?
            esc(p.owner.name || '?') + ' <code>PID ' + (p.owner.pid || '?') + '</code>' :
            '';
        const actions =
            '<button class="c-btn" data-act="suggest" data-id="' + p.serviceId + '" data-from="' + p.port + '">' + iconHTML('refresh') + ' ' + t('ports.suggest') + '</button>' +
            '<button class="c-btn" data-act="edit"    data-id="' + p.serviceId + '" data-name="' + esc(p.serviceName) + '">' + iconHTML('settings') + ' ' + t('ports.edit') + '</button>';
        return '<tr class="' + rowClass + '">' +
            '<td><strong>' + esc(p.serviceName) + '</strong><br/><small style="color:var(--color-text-muted);">' + esc(p.serviceId) + '</small></td>' +
            '<td><code>' + p.port + '</code></td>' +
            '<td>' + badge + '</td>' +
            '<td>' + owner + '</td>' +
            '<td class="c-table__actions">' + actions + '</td>' +
        '</tr>';
    }).join('');

    root.innerHTML = help +
        '<table class="c-table"><thead><tr>' +
            '<th>' + t('ports.col.service') + '</th>' +
            '<th>' + t('ports.col.desired') + '</th>' +
            '<th>' + t('ports.col.status')  + '</th>' +
            '<th>' + t('ports.col.owner')   + '</th>' +
            '<th>' + t('ports.col.actions') + '</th>' +
        '</tr></thead><tbody>' + rows + '</tbody></table>';

    root.querySelectorAll('button[data-act]').forEach(btn => {
        btn.addEventListener('click', async () => {
            const act = btn.dataset.act, id = btn.dataset.id;
            if (act === 'suggest') {
                const from = parseInt(btn.dataset.from, 10) + 1;
                const next = await window.go.main.App.SuggestFreePort(from);
                if (!next) { alert(t('ports.noFree')); return; }
                await window.go.main.App.SetPortOverride(id, next);
                alert(t('ports.suggestApplied', next));
                paintPorts(root);
            } else if (act === 'edit') {
                const name = btn.dataset.name;
                const v = prompt(t('ports.editPrompt', name));
                if (!v) return;
                const n = parseInt(v, 10);
                if (!n || n < 1024 || n > 65534) return;
                try {
                    await window.go.main.App.SetPortOverride(id, n);
                    alert(t('ports.editApplied'));
                    paintPorts(root);
                } catch (e) { alert('Error: ' + e); }
            }
        });
    });
}

/* ---------- Consoles tab -------------------------------------------------- */
let consoleAutoscroll = true;
let consoleFilter = 'all';
let consoleSearch = '';

async function renderConsoles(root) {
    if (!STATE.services.length) {
        root.innerHTML = '<div class="c-placeholder">' + t('consoles.noService') + '</div>';
        return;
    }
    if (!STATE.activeConsole) STATE.activeConsole = STATE.services[0].id;

    // Header actions
    document.getElementById('viewActions').innerHTML =
        '<button class="c-btn" id="actClear">'  + iconHTML('trash') + ' ' + t('consoles.clear') + '</button>' +
        '<button class="c-btn" id="actCopy">'   + iconHTML('copy')  + ' ' + t('consoles.copy')  + '</button>';
    document.getElementById('actClear').addEventListener('click', clearActiveConsole);
    document.getElementById('actCopy').addEventListener('click', copyActiveConsole);

    // Service subtabs + filter bar + log pane
    const tabs = STATE.services.map(svc =>
        '<button class="c-subtab' + (svc.id === STATE.activeConsole ? ' is-active' : '') + '" data-id="' + svc.id + '">' +
            esc(svc.name) +
        '</button>'
    ).join('');

    root.innerHTML =
        '<div class="c-console">' +
            '<div class="c-subtabs">' + tabs + '</div>' +
            '<div class="c-console__toolbar">' +
                '<input type="text" class="c-input" id="consoleSearch" placeholder="' + t('consoles.search') + '" value="' + esc(consoleSearch) + '"/>' +
                '<select class="c-input" id="consoleFilter">' +
                    '<option value="all"   ' + (consoleFilter==='all'?'selected':'') + '>' + t('consoles.filter.all')   + '</option>' +
                    '<option value="warn"  ' + (consoleFilter==='warn'?'selected':'') + '>' + t('consoles.filter.warn')  + '</option>' +
                    '<option value="error" ' + (consoleFilter==='error'?'selected':'') + '>' + t('consoles.filter.error') + '</option>' +
                '</select>' +
                '<label class="c-checkbox"><input type="checkbox" id="consoleAuto" ' + (consoleAutoscroll?'checked':'') + '/> ' + t('consoles.autoscroll') + '</label>' +
            '</div>' +
            '<pre class="c-console__pane" id="consolePane"></pre>' +
        '</div>';

    root.querySelectorAll('.c-subtab').forEach(b => b.addEventListener('click', () => {
        STATE.activeConsole = b.dataset.id;
        renderConsoles(root);
    }));
    document.getElementById('consoleSearch').addEventListener('input', e => { consoleSearch = e.target.value; renderConsolePane(); });
    document.getElementById('consoleFilter').addEventListener('change', e => { consoleFilter = e.target.value; renderConsolePane(); });
    document.getElementById('consoleAuto').addEventListener('change', e => { consoleAutoscroll = e.target.checked; });

    // Load snapshot from backend, then render.
    if (!STATE.logs[STATE.activeConsole]) {
        try {
            const lines = await window.go.main.App.GetServiceLogs(STATE.activeConsole);
            STATE.logs[STATE.activeConsole] = lines || [];
        } catch (e) {
            STATE.logs[STATE.activeConsole] = [];
        }
    }
    renderConsolePane();
}

function renderConsolePane() {
    const pane = document.getElementById('consolePane');
    if (!pane) return;
    const lines = STATE.logs[STATE.activeConsole] || [];
    if (!lines.length) { pane.innerHTML = '<span style="color:var(--color-text-muted);">' + t('consoles.empty') + '</span>'; return; }

    const filtered = lines.filter(filterLine);
    pane.innerHTML = filtered.map(formatLineHTML).join('\n');
    if (consoleAutoscroll) pane.scrollTop = pane.scrollHeight;
}

function appendConsoleLine(line) {
    const pane = document.getElementById('consolePane');
    if (!pane) return;
    if (!filterLine(line)) return;
    if (pane.children.length === 1 && pane.firstChild && pane.firstChild.nodeName === 'SPAN' && !pane.firstChild.dataset) {
        pane.innerHTML = '';
    }
    pane.insertAdjacentHTML('beforeend', formatLineHTML(line) + '\n');
    if (consoleAutoscroll) pane.scrollTop = pane.scrollHeight;
}

function filterLine(line) {
    if (consoleFilter === 'error' && line.l !== 'ERROR') return false;
    if (consoleFilter === 'warn' && line.l !== 'ERROR' && line.l !== 'WARN') return false;
    if (consoleSearch && !line.x.toLowerCase().includes(consoleSearch.toLowerCase())) return false;
    return true;
}

function formatLineHTML(line) {
    const ts = new Date(line.t).toISOString().slice(11, 23);
    const cls = 'c-log--' + (line.l || 'INFO').toLowerCase();
    return '<span class="c-log__ts">' + ts + '</span> ' +
           '<span class="c-log__lv ' + cls + '">' + (line.l || 'INFO').padEnd(5) + '</span> ' +
           '<span class="c-log__text">' + esc(line.x) + '</span>';
}

function clearActiveConsole() {
    if (!STATE.activeConsole) return;
    try { window.go.main.App.ClearServiceLogs(STATE.activeConsole); } catch (e) {}
    STATE.logs[STATE.activeConsole] = [];
    renderConsolePane();
}
function copyActiveConsole() {
    const lines = STATE.logs[STATE.activeConsole] || [];
    const text = lines.map(l => new Date(l.t).toISOString().slice(11, 23) + ' ' + (l.l || 'INFO') + ' ' + l.x).join('\n');
    navigator.clipboard && navigator.clipboard.writeText(text);
}

/* ---------- helpers ------------------------------------------------------- */
function card(title, body, extraStyle) {
    const style = extraStyle ? (' style="' + extraStyle + '"') : '';
    return '<div class="c-card"' + style + '>' +
        '<div class="c-card__title">' + esc(title) + '</div>' +
        '<div class="c-card__body">' + body + '</div>' +
    '</div>';
}

function statusBadge(st) {
    if (!st || !st.running) return '<span class="c-badge c-badge--muted">' + t('services.status.stopped') + '</span>';
    if (st.healthy)         return '<span class="c-badge c-badge--ok">'   + iconHTML('check') + ' ' + t('services.status.healthy') + '</span>';
    return '<span class="c-badge c-badge--warn">' + t('services.status.running') + '</span>';
}

function esc(s) {
    return String(s ?? '').replace(/[&<>"']/g, c => ({
        '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'
    }[c]));
}
