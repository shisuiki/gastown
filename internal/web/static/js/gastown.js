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
            '<td><a href="/agent/' + escapeHtml(a.SessionID) + '">' + escapeHtml(a.Name) + '</a></td>' +
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
    return '<table><tr><th>ID</th><th>Title</th><th>Priority</th><th>Type</th></tr>' +
        issues.map(i =>
            '<tr><td><a href="/bead/' + escapeHtml(i.id) + '"><code class="text-link">' + escapeHtml(i.id) + '</code></a></td>' +
            '<td>' + escapeHtml(i.title) + '</td>' +
            '<td>' + priorityBadge(i.priority) + '</td>' +
            '<td>' + issueTypeIcon(i.issue_type) + '</td></tr>'
        ).join('') + '</table>';
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
// Claude Usage Monitoring
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

/**
 * Shorten model name for display
 * @param {string} model - Full model name
 * @returns {string} Short model name
 */
function shortModelName(model) {
    return model
        .replace('claude-', '')
        .replace('deepseek-', 'ds-')
        .replace('-20251101', '')
        .replace('-20250929', '')
        .replace('-20251001', '');
}

/**
 * Render Claude usage info card content
 * @param {object} usage - Usage info from /api/claude/usage
 * @returns {string} HTML content
 */
function renderClaudeUsage(usage) {
    if (!usage || usage.error) {
        return '<p class="text-muted">' + escapeHtml(usage?.error || 'Unable to load Claude usage') + '</p>';
    }

    let html = '<div class="system-stats">';

    // Today's usage
    if (usage.today) {
        html += '<div class="sys-row"><span class="sys-label">Today</span><span class="sys-value">' +
            formatCost(usage.today.total_cost) + '</span></div>';
        html += '<div class="sys-row"><span class="sys-label">Tokens</span><span class="sys-value">' +
            formatTokens(usage.today.total_tokens) + '</span></div>';

        // Model breakdown for today
        if (usage.today.models && usage.today.models.length > 0) {
            for (const m of usage.today.models) {
                if (m.cost > 0.01) {  // Only show models with >$0.01 usage
                    html += '<div class="sys-row"><span class="sys-label" style="padding-left:10px">' +
                        shortModelName(m.model) + '</span><span class="sys-value">' +
                        formatCost(m.cost) + '</span></div>';
                }
            }
        }
    }

    // Active billing block
    if (usage.active_block) {
        html += '<div class="sys-section" style="margin-top:8px;padding-top:8px;border-top:1px solid #333">' +
            '<span class="sys-label">Current Block</span><span class="sys-value">' +
            formatCost(usage.active_block.total_cost) + '</span></div>';
        if (usage.active_block.burn_rate > 0) {
            html += '<div class="sys-row"><span class="sys-label">Burn Rate</span><span class="sys-value">' +
                formatCost(usage.active_block.burn_rate) + '/hr</span></div>';
        }
        if (usage.active_block.projected_cost > 0) {
            html += '<div class="sys-row"><span class="sys-label">Projected</span><span class="sys-value">' +
                formatCost(usage.active_block.projected_cost) + '</span></div>';
        }
    }

    // Total usage
    if (usage.totals) {
        html += '<div class="sys-section" style="margin-top:8px;padding-top:8px;border-top:1px solid #333">' +
            '<span class="sys-label">All Time</span><span class="sys-value">' +
            formatCost(usage.totals.total_cost) + '</span></div>';
    }

    html += '</div>';
    return html;
}

/**
 * Fetch and update Claude usage display
 * @param {string} elementId - ID of element to update
 */
async function loadClaudeUsage(elementId) {
    try {
        const res = await fetch('/api/claude/usage');
        const data = await res.json();
        const el = document.getElementById(elementId);
        if (el) {
            el.innerHTML = renderClaudeUsage(data);
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
