/**
 * Gas Town WebUI - Shared Utilities
 * Common functions used across all pages
 */

// ============================================================================
// HTML/String Utilities
// ============================================================================

/**
 * Escape HTML to prevent XSS
 * @param {string} text - Raw text to escape
 * @returns {string} HTML-safe string
 */
function escapeHtml(text) {
    if (text === null || text === undefined) return '';
    const div = document.createElement('div');
    div.textContent = String(text);
    return div.innerHTML;
}

/**
 * Get CSS class for activity/status colors
 * @param {string} color - Color identifier from backend
 * @returns {string} CSS class name
 */
function getColorClass(color) {
    if (color === 'green' || color === 'activity-green') return 'green';
    if (color === 'yellow' || color === 'activity-yellow') return 'yellow';
    if (color === 'red' || color === 'activity-red') return 'red';
    return 'blue';
}

/**
 * Get emoji icon for agent type
 * @param {string} type - Agent type (crew, polecat, refinery, patrol)
 * @returns {string} Emoji icon
 */
function agentTypeIcon(type) {
    switch (type) {
        case 'crew': return '👷';
        case 'refinery': return '🏭';
        case 'polecat': return '🐱';
        case 'mayor': return '🏛️';
        case 'deacon': return '⛪';
        case 'witness': return '👁️';
        case 'boot': return '🥾';
        case 'dog': return '🐕';
        case 'patrol': return '🛡️';
        default: return '🤖';
    }
}

/**
 * Get badge HTML for priority
 * @param {number} priority - Priority level (1-4)
 * @returns {string} HTML badge element
 */
function priorityBadge(p) {
    const colors = { 1: 'red', 2: 'yellow', 3: 'blue', 4: 'green' };
    return '<span class="badge badge-' + (colors[p] || 'blue') + '">P' + p + '</span>';
}

/**
 * Get emoji icon for issue type
 * @param {string} type - Issue type
 * @returns {string} Emoji icon
 */
function issueTypeIcon(t) {
    const icons = { 'task': '📋', 'bug': '🐛', 'feature': '✨', 'doc': '📖' };
    return icons[t] || '📌';
}

// ============================================================================
// Navigation Utilities
// ============================================================================

/**
 * Toggle mobile navigation menu
 */
function toggleNav() {
    const navLinks = document.getElementById('nav-links');
    if (navLinks) {
        navLinks.classList.toggle('open');
    }
}

/**
 * Set active navigation link based on current path
 */
function setActiveNavLink() {
    const path = window.location.pathname;
    const links = document.querySelectorAll('.nav-link');
    links.forEach(link => {
        const href = link.getAttribute('href');
        if (href === path || (path === '/' && href === '/')) {
            link.classList.add('active');
        } else {
            link.classList.remove('active');
        }
    });
}

// ============================================================================
// WebSocket/SSE Utilities with Reconnection
// ============================================================================

/**
 * Reconnecting WebSocket wrapper
 * Automatically reconnects with exponential backoff
 */
class ReconnectingWebSocket {
    constructor(url, options = {}) {
        this.url = url;
        this.maxReconnectDelay = options.maxReconnectDelay || 30000;
        this.reconnectAttempts = 0;
        this.listeners = {
            open: [],
            message: [],
            close: [],
            error: []
        };
        this.connect();
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss://' : 'ws://';
        this.socket = new WebSocket(protocol + window.location.host + this.url);

        this.socket.addEventListener('open', (e) => {
            this.reconnectAttempts = 0;
            this.listeners.open.forEach(fn => fn(e));
        });

        this.socket.addEventListener('message', (e) => {
            this.listeners.message.forEach(fn => fn(e));
        });

        this.socket.addEventListener('close', (e) => {
            this.listeners.close.forEach(fn => fn(e));
            this.scheduleReconnect();
        });

        this.socket.addEventListener('error', (e) => {
            this.listeners.error.forEach(fn => fn(e));
        });
    }

    scheduleReconnect() {
        this.reconnectAttempts++;
        const baseDelay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), this.maxReconnectDelay);
        const jitter = Math.random() * 1000;
        const delay = baseDelay + jitter;
        console.log(`WebSocket reconnecting in ${Math.round(delay)}ms (attempt ${this.reconnectAttempts})`);
        setTimeout(() => this.connect(), delay);
    }

    on(event, callback) {
        if (this.listeners[event]) {
            this.listeners[event].push(callback);
        }
    }

    send(data) {
        if (this.socket && this.socket.readyState === WebSocket.OPEN) {
            this.socket.send(data);
        }
    }

    close() {
        if (this.socket) {
            this.socket.close();
        }
    }
}

/**
 * Reconnecting EventSource wrapper for SSE
 * Handles transient errors and session-ended events
 */
class ReconnectingEventSource {
    constructor(url, options = {}) {
        this.url = url;
        this.maxReconnectDelay = options.maxReconnectDelay || 30000;
        this.reconnectAttempts = 0;
        this.stopped = false;
        this.listeners = {};
        this.source = null;
        this.onStateChange = options.onStateChange || (() => {});
    }

    connect() {
        if (this.stopped) return;

        this.source = new EventSource(this.url);
        this.onStateChange('connecting');

        this.source.addEventListener('open', () => {
            this.reconnectAttempts = 0;
            this.onStateChange('connected');
        });

        // Re-attach all registered listeners
        Object.entries(this.listeners).forEach(([event, callbacks]) => {
            callbacks.forEach(cb => {
                this.source.addEventListener(event, cb);
            });
        });

        this.source.addEventListener('error', (e) => {
            const data = e.data || '';
            if (data.startsWith('session_ended:')) {
                this.onStateChange('ended', data.substring(14));
                this.stop();
            } else if (data === 'max_errors_reached') {
                this.onStateChange('error', 'Too many errors');
                this.stop();
            } else if (!data.startsWith('transient:')) {
                // Generic error - schedule reconnect
                this.onStateChange('reconnecting');
                this.scheduleReconnect();
            }
        });
    }

    scheduleReconnect() {
        if (this.stopped) return;
        this.reconnectAttempts++;
        const baseDelay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), this.maxReconnectDelay);
        const jitter = Math.random() * 1000;
        const delay = baseDelay + jitter;
        console.log(`SSE reconnecting in ${Math.round(delay)}ms (attempt ${this.reconnectAttempts})`);
        setTimeout(() => this.connect(), delay);
    }

    on(event, callback) {
        if (!this.listeners[event]) {
            this.listeners[event] = [];
        }
        this.listeners[event].push(callback);

        // If already connected, add listener immediately
        if (this.source) {
            this.source.addEventListener(event, callback);
        }
    }

    stop() {
        this.stopped = true;
        if (this.source) {
            this.source.close();
            this.source = null;
        }
        this.onStateChange('disconnected');
    }

    start() {
        this.stopped = false;
        this.connect();
    }
}

// ============================================================================
// API Utilities
// ============================================================================

function getCookie(name) {
    const match = document.cookie.match(new RegExp('(^|;\\s*)' + name + '=([^;]*)'));
    return match ? decodeURIComponent(match[2]) : '';
}

function getCSRFToken() {
    return getCookie('gt_csrf');
}

function isStateChangingMethod(method) {
    return ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase());
}

function isSameOriginUrl(url) {
    if (url.startsWith('/')) return true;
    try {
        const parsed = new URL(url, window.location.origin);
        return parsed.origin === window.location.origin;
    } catch (err) {
        return false;
    }
}

const originalFetch = window.fetch.bind(window);
window.fetch = (input, init = {}) => {
    let url = '';
    let method = 'GET';
    let headers = new Headers();

    if (input instanceof Request) {
        url = input.url;
        method = input.method || method;
        headers = new Headers(input.headers);
    } else if (typeof input === 'string') {
        url = input;
    }

    if (init.method) {
        method = init.method;
    }
    if (init.headers) {
        headers = new Headers(init.headers);
    }

    if (isStateChangingMethod(method) && isSameOriginUrl(url)) {
        const token = getCSRFToken();
        if (token && !headers.has('X-CSRF-Token')) {
            headers.set('X-CSRF-Token', token);
        }
    }

    const finalInit = { ...init, headers };

    if (input instanceof Request) {
        return originalFetch(new Request(input, finalInit));
    }

    return originalFetch(input, finalInit);
};

/**
 * Fetch JSON from API endpoint
 * @param {string} url - API URL
 * @param {object} options - Fetch options
 * @returns {Promise<object>} JSON response
 */
async function fetchJSON(url, options = {}) {
    const response = await fetch(url, {
        headers: {
            'Content-Type': 'application/json',
            ...options.headers
        },
        ...options
    });
    return response.json();
}

/**
 * POST JSON to API endpoint
 * @param {string} url - API URL
 * @param {object} data - Data to send
 * @returns {Promise<object>} JSON response
 */
async function postJSON(url, data) {
    return fetchJSON(url, {
        method: 'POST',
        body: JSON.stringify(data)
    });
}

// ============================================================================
// Table/List Rendering Utilities
// ============================================================================

/**
 * Render an agents table
 * @param {Array} agents - Array of agent objects
 * @returns {string} HTML table
 */
function renderAgentsTable(agents) {
    if (!agents || agents.length === 0) {
        return '<p class="text-muted">No agents running</p>';
    }
    return '<table><tr><th>Type</th><th>Name</th><th>Rig</th><th>Activity</th></tr>' +
        agents.map(a =>
            '<tr><td>' + agentTypeIcon(a.AgentType) + '</td>' +
            '<td><a href="/terminals?session=' + encodeURIComponent(a.SessionID) + '">' + escapeHtml(a.Name) + '</a></td>' +
            '<td>' + escapeHtml(a.Rig) + '</td>' +
            '<td><span class="badge badge-' + getColorClass(a.LastActivity.ColorClass) + '">' +
            escapeHtml(a.LastActivity.FormattedAge) + '</span></td></tr>'
        ).join('') + '</table>';
}

/**
 * Render a convoys table
 * @param {Array} convoys - Array of convoy objects
 * @returns {string} HTML table
 */
function renderConvoysTable(convoys) {
    if (!convoys || convoys.length === 0) {
        return '<p class="text-muted">No active convoys</p>';
    }
    return '<table><tr><th>ID</th><th>Title</th><th>Progress</th></tr>' +
        convoys.map(c =>
            '<tr><td><a href="/convoy/' + escapeHtml(c.ID) + '">' + escapeHtml(c.ID) + '</a></td>' +
            '<td>' + escapeHtml(c.Title) + '</td>' +
            '<td>' + escapeHtml(c.Progress) + '</td></tr>'
        ).join('') + '</table>';
}

/**
 * Render a rigs table
 * @param {Array} rigs - Array of rig objects
 * @returns {string} HTML table
 */
function renderRigsTable(rigs) {
    if (!rigs || rigs.length === 0) {
        return '<p class="text-muted">No rigs configured</p>';
    }
    return '<table><tr><th>Name</th><th>Agents</th><th>Crew</th></tr>' +
        rigs.map(r =>
            '<tr><td>' + escapeHtml(r.name) + '</td>' +
            '<td>' + (r.agents || r.polecats || 0) + '</td>' +
            '<td>' + (r.crew || 0) + '</td></tr>'
        ).join('') + '</table>';
}

/**
 * Render an issues table
 * @param {Array} issues - Array of issue objects
 * @returns {string} HTML table
 */
function renderIssuesTable(issues) {
    if (!issues || issues.length === 0) {
        return '<p class="text-muted">No open issues</p>';
    }

    // Separate wisp issues from work issues
    const wispIssues = issues.filter(i => i.wisp);
    const workIssues = issues.filter(i => !i.wisp);

    // Sort wisp issues: pinned first
    wispIssues.sort((a, b) => (a.status === 'pinned' ? -1 : b.status === 'pinned' ? 1 : 0));

    let html = '';

    // Render wisp workflow issues section
    if (wispIssues.length > 0) {
        html += '<div style="margin-bottom: 20px;">';
        html += '<h3 style="font-size: 0.9rem; color: #94a3b8; margin-bottom: 8px;">📌 Wisp Workflow Issues</h3>';
        html += '<table><tr><th>ID</th><th>Title</th><th>Assignee</th><th>Priority</th><th>Type</th><th>Status</th></tr>' +
            wispIssues.map(i => {
                let statusIndicator = '';
                if (i.status === 'pinned') {
                    statusIndicator = '📌 <span class="text-muted">pinned</span>';
                } else if (i.status === 'open') {
                    statusIndicator = '<span class="text-muted">open</span>';
                } else if (i.status === 'in_progress') {
                    statusIndicator = '⚙️ <span class="text-muted">in progress</span>';
                } else {
                    statusIndicator = '<span class="text-muted">' + escapeHtml(i.status) + '</span>';
                }
                return '<tr class="wisp-row">' +
                    '<td><a href="/bead/' + escapeHtml(i.id) + '"><code class="text-link">' + escapeHtml(i.id) + '</code></a></td>' +
                    '<td>' + escapeHtml(i.title) + '</td>' +
                    '<td>' + (i.assignee ? escapeHtml(i.assignee) : '<span class="text-muted">—</span>') + '</td>' +
                    '<td>' + priorityBadge(i.priority) + '</td>' +
                    '<td>' + issueTypeIcon(i.issue_type) + ' <span class="wisp-indicator" title="Wisp">✨</span></td>' +
                    '<td>' + statusIndicator + '</td></tr>';
            }).join('') + '</table>';
        html += '</div>';
    }

    // Render active work assignments section
    if (workIssues.length > 0) {
        html += '<div>';
        html += '<h3 style="font-size: 0.9rem; color: #94a3b8; margin-bottom: 8px;">📋 Active Work Assignments</h3>';
        html += '<table><tr><th>ID</th><th>Title</th><th>Assignee</th><th>Priority</th><th>Type</th><th>Status</th></tr>' +
            workIssues.map(i => {
                let statusIndicator = '';
                if (i.status === 'pinned') {
                    statusIndicator = '📌 <span class="text-muted">pinned</span>';
                } else if (i.status === 'open') {
                    statusIndicator = '<span class="text-muted">open</span>';
                } else if (i.status === 'in_progress') {
                    statusIndicator = '⚙️ <span class="text-muted">in progress</span>';
                } else {
                    statusIndicator = '<span class="text-muted">' + escapeHtml(i.status) + '</span>';
                }
                return '<tr>' +
                    '<td><a href="/bead/' + escapeHtml(i.id) + '"><code class="text-link">' + escapeHtml(i.id) + '</code></a></td>' +
                    '<td>' + escapeHtml(i.title) + '</td>' +
                    '<td>' + (i.assignee ? escapeHtml(i.assignee) : '<span class="text-muted">—</span>') + '</td>' +
                    '<td>' + priorityBadge(i.priority) + '</td>' +
                    '<td>' + issueTypeIcon(i.issue_type) + '</td>' +
                    '<td>' + statusIndicator + '</td></tr>';
            }).join('') + '</table>';
        html += '</div>';
    }

    // If all issues are filtered out (shouldn't happen)
    if (html === '') {
        return '<p class="text-muted">No open issues</p>';
    }

    return html;
}

/**
 * Render a commits/activity table
 * @param {Array} commits - Array of commit objects
 * @returns {string} HTML table
 */
function renderCommitsTable(commits) {
    if (!commits || commits.length === 0) {
        return '<p class="text-muted">No recent commits</p>';
    }
    return '<table><tr><th>Hash</th><th>Message</th><th>Author</th><th>Age</th></tr>' +
        commits.map(c =>
            '<tr><td><code class="text-link">' + escapeHtml(c.hash) + '</code></td>' +
            '<td>' + escapeHtml(c.message) + '</td>' +
            '<td class="text-secondary">' + escapeHtml(c.author) + '</td>' +
            '<td class="text-muted">' + escapeHtml(c.age) + '</td></tr>'
        ).join('') + '</table>';
}

// ============================================================================
// Version Management
// ============================================================================

/**
 * Fetch and display the current version in the navbar badge
 */
async function loadVersion() {
    try {
        const res = await fetch('/api/version');
        const data = await res.json();
        const badge = document.getElementById('version-badge');
        if (badge && data.version) {
            badge.textContent = 'v' + data.version;
        }
    } catch (e) {
        console.error('Failed to load version:', e);
    }
}

// ============================================================================
// System Monitoring
// ============================================================================

/**
 * Render system info card content
 * @param {object} info - System info from /api/system
 * @returns {string} HTML content
 */
function renderSystemInfo(info) {
    if (!info) {
        return '<p class="text-muted">Unable to load system info</p>';
    }

    const memBar = info.mem_percent ?
        '<div class="progress-bar"><div class="progress-fill" style="width: ' + info.mem_percent.toFixed(1) + '%"></div></div>' : '';
    const diskBar = info.disk_percent ?
        '<div class="progress-bar"><div class="progress-fill" style="width: ' + info.disk_percent.toFixed(1) + '%"></div></div>' : '';

    // Build version string with build number
    let versionStr = info.version || '-';
    if (info.build_number && info.build_number !== 'dev') {
        versionStr += '.' + info.build_number;
    }
    if (info.build_commit && info.build_commit !== 'unknown') {
        versionStr += ' <span class="text-muted" style="font-size: 0.75rem;">(' + escapeHtml(info.build_commit) + ')</span>';
    }

    return '<div class="system-stats">' +
        '<div class="sys-row"><span class="sys-label">Version</span><span class="sys-value">' + versionStr + '</span></div>' +
        '<div class="sys-row"><span class="sys-label">Deployed</span><span class="sys-value">' + escapeHtml(info.build_time || '-') + '</span></div>' +
        '<div class="sys-row"><span class="sys-label">Service Up</span><span class="sys-value">' + escapeHtml(info.service_uptime || '-') + '</span></div>' +
        '<div class="sys-row"><span class="sys-label">Host</span><span class="sys-value">' + escapeHtml(info.hostname || '-') + '</span></div>' +
        '<div class="sys-row"><span class="sys-label">OS</span><span class="sys-value">' + escapeHtml(info.os + '/' + info.arch) + '</span></div>' +
        '<div class="sys-row"><span class="sys-label">Uptime</span><span class="sys-value">' + escapeHtml(info.uptime || '-') + '</span></div>' +
        '<div class="sys-row"><span class="sys-label">Load</span><span class="sys-value">' + escapeHtml(info.load_avg || '-') + '</span></div>' +
        '<div class="sys-section"><span class="sys-label">Memory</span><span class="sys-value">' +
            escapeHtml(info.mem_used || '-') + ' / ' + escapeHtml(info.mem_total || '-') +
            ' (' + (info.mem_percent ? info.mem_percent.toFixed(1) : '-') + '%)</span>' + memBar + '</div>' +
        '<div class="sys-section"><span class="sys-label">Disk</span><span class="sys-value">' +
            escapeHtml(info.disk_used || '-') + ' / ' + escapeHtml(info.disk_total || '-') +
            ' (' + (info.disk_percent ? info.disk_percent.toFixed(1) : '-') + '%)</span>' + diskBar + '</div>' +
        '</div>';
}

/**
 * Fetch and update system info display
 * @param {string} elementId - ID of element to update
 */
async function loadSystemInfo(elementId) {
    try {
        const res = await fetch('/api/system');
        const data = await res.json();
        const el = document.getElementById(elementId);
        if (el) {
            el.innerHTML = renderSystemInfo(data);
        }
    } catch (e) {
        const el = document.getElementById(elementId);
        if (el) {
            el.innerHTML = '<p class="text-danger">Error: ' + escapeHtml(e.message) + '</p>';
        }
    }
}

// ============================================================================
// CLI Usage Monitoring
// ============================================================================

/**
 * Format large numbers with K/M suffix
 * @param {number} num - Number to format
 * @returns {string} Formatted string
 */
function formatTokens(num) {
    if (!num) return '0';
    if (num >= 1000000000) return (num / 1000000000).toFixed(1) + 'B';
    if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
    if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
    return num.toString();
}

/**
 * Format cost in USD
 * @param {number} cost - Cost in USD
 * @returns {string} Formatted cost
 */
function formatCost(cost) {
    if (!cost) return '$0.00';
    return '$' + cost.toFixed(2);
}

function formatUsageValue(row) {
    const parts = [];
    if (row.tokens !== null && row.tokens !== undefined) {
        parts.push(formatTokens(Math.round(row.tokens)) + ' tok');
    }
    if (row.cost !== null && row.cost !== undefined) {
        parts.push(formatCost(row.cost));
    }
    if (parts.length === 0) {
        return '-';
    }
    return parts.join(' · ');
}

/**
 * Render CLI usage info card content
 * @param {object} summary - Usage info from /api/cli/usage
 * @returns {string} HTML content
 */
function renderCLIUsage(summary) {
    if (!summary || !summary.providers) {
        return '<p class="text-muted">Unable to load CLI usage</p>';
    }

    let html = '<div class="system-stats">';
    for (const provider of summary.providers) {
        html += '<div class="sys-section"><span class="sys-label">' +
            escapeHtml(provider.provider || 'Unknown') + '</span><span class="sys-value"></span></div>';

        if (provider.error) {
            html += '<div class="sys-row"><span class="sys-label" style="padding-left:10px">Status</span><span class="sys-value">' +
                escapeHtml(provider.error) + '</span></div>';
            continue;
        }

        if (!provider.rows || provider.rows.length === 0) {
            html += '<div class="sys-row"><span class="sys-label" style="padding-left:10px">Status</span><span class="sys-value">No data</span></div>';
            continue;
        }

        for (const row of provider.rows) {
            html += '<div class="sys-row"><span class="sys-label" style="padding-left:10px">' +
                escapeHtml(row.label || 'Usage') + '</span><span class="sys-value">' +
                formatUsageValue(row) + '</span></div>';
        }
    }
    html += '</div>';
    return html;
}

/**
 * Fetch and update CLI usage display
 * @param {string} elementId - ID of element to update
 */
async function loadCLIUsage(elementId) {
    try {
        const res = await fetch('/api/cli/usage');
        const data = await res.json();
        const el = document.getElementById(elementId);
        if (el) {
            el.innerHTML = renderCLIUsage(data);
        }
    } catch (e) {
        const el = document.getElementById(elementId);
        if (el) {
            el.innerHTML = '<p class="text-danger">Error: ' + escapeHtml(e.message) + '</p>';
        }
    }
}

function progressClass(percent) {
    if (percent >= 90) return 'danger';
    if (percent >= 75) return 'warning';
    return '';
}

function renderCLILimits(limits) {
    if (!limits || !limits.providers) {
        return '<p class="text-muted">Unable to load limits</p>';
    }

    let html = '<div class="system-stats">';
    for (const provider of limits.providers) {
        const period = provider.period || 'weekly';
        const label = provider.provider ? (provider.provider + ' (' + period + ')') : 'Unknown';
        let percent = provider.percent;
        if ((percent === null || percent === undefined) && provider.used !== null && provider.used !== undefined &&
            provider.limit !== null && provider.limit !== undefined && provider.limit > 0) {
            percent = (provider.used / provider.limit) * 100;
        }

        const percentValue = percent !== null && percent !== undefined ? Math.min(Math.max(percent, 0), 100) : null;
        const percentLabel = percentValue !== null ? percentValue.toFixed(1) + '%' : '-';
        html += '<div class="sys-section"><span class="sys-label">' + escapeHtml(label) + '</span>' +
            '<span class="sys-value">' + percentLabel + '</span></div>';

        if (provider.error) {
            html += '<div class="sys-row"><span class="sys-label" style="padding-left:10px">Status</span><span class="sys-value">' +
                escapeHtml(provider.error) + '</span></div>';
            continue;
        }

        if (provider.used !== null && provider.used !== undefined) {
            const unit = provider.unit || '';
            const usedLabel = formatLimitValue(provider.used, unit);
            let limitLabel = '';
            if (provider.limit !== null && provider.limit !== undefined && provider.limit > 0) {
                limitLabel = ' / ' + formatLimitValue(provider.limit, unit);
            }
            html += '<div class="sys-row"><span class="sys-label" style="padding-left:10px">Used</span><span class="sys-value">' +
                usedLabel + limitLabel + '</span></div>';
        }

        if (percentValue !== null) {
            const fillClass = progressClass(percentValue);
            html += '<div class="progress-bar"><div class="progress-fill ' + fillClass + '" style="width: ' +
                percentValue.toFixed(1) + '%;"></div></div>';
        }
    }
    html += '</div>';
    return html;
}

function formatLimitValue(value, unit) {
    if (value === null || value === undefined) {
        return '-';
    }
    if (unit.toLowerCase() === 'usd') {
        return formatCost(value);
    }
    if (unit.toLowerCase() === 'tokens') {
        return formatTokens(Math.round(value)) + ' tok';
    }
    return value.toFixed(2) + (unit ? (' ' + unit) : '');
}

async function loadCLILimits(elementId) {
    try {
        const res = await fetch('/api/cli/limits');
        const data = await res.json();
        const el = document.getElementById(elementId);
        if (el) {
            el.innerHTML = renderCLILimits(data);
        }
    } catch (e) {
        const el = document.getElementById(elementId);
        if (el) {
            el.innerHTML = '<p class="text-danger">Error: ' + escapeHtml(e.message) + '</p>';
        }
    }
}

// ============================================================================
// Initialize on page load
// ============================================================================

document.addEventListener('DOMContentLoaded', function() {
    setActiveNavLink();
    loadVersion();
});
