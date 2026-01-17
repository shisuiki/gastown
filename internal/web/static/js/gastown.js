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
    return '<table><tr><th>Name</th><th>Polecats</th><th>Crew</th></tr>' +
        rigs.map(r =>
            '<tr><td>' + escapeHtml(r.name) + '</td>' +
            '<td>' + (r.polecats || 0) + '</td>' +
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
// Initialize on page load
// ============================================================================

document.addEventListener('DOMContentLoaded', function() {
    setActiveNavLink();
});
