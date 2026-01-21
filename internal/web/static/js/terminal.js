/**
 * Gas Town WebUI - Terminal Widget
 * Unified terminal component for streaming tmux output
 */

/**
 * TerminalWidget - Reusable terminal viewer with SSE streaming
 *
 * Usage:
 *   const terminal = new TerminalWidget({
 *     container: '#terminal-container',
 *     outputEl: '#terminal-output',
 *     toggleBtn: '#terminal-toggle',
 *     inputEl: '#terminal-input',       // optional
 *     inputArea: '#terminal-input-area', // optional
 *     sessionInfoEl: '#session-info',    // optional
 *     sessionId: 'gt-rig-agent',         // or null for selection mode
 *     sessionSelectEl: '#terminal-session', // optional, for selection mode
 *     streamUrl: '/api/terminal/stream', // SSE endpoint
 *     sendUrl: '/api/terminal/send',     // POST endpoint for input
 *   });
 *   terminal.connect();
 */
class TerminalWidget {
    constructor(options) {
        this.options = Object.assign({
            streamUrl: '/api/terminal/stream',
            sendUrl: '/api/terminal/send',
            showInput: true,
            showSessionInfo: true,
            sendEnter: true,
            onSend: null,
            onSendKey: null,
            targetLabel: null,
        }, options);

        // Get DOM elements
        this.container = document.querySelector(this.options.container);
        this.outputEl = document.querySelector(this.options.outputEl);
        this.toggleBtn = document.querySelector(this.options.toggleBtn);
        this.inputEl = this.options.inputEl ? document.querySelector(this.options.inputEl) : null;
        this.inputArea = this.options.inputArea ? document.querySelector(this.options.inputArea) : null;
        this.sessionInfoEl = this.options.sessionInfoEl ? document.querySelector(this.options.sessionInfoEl) : null;
        this.sessionSelectEl = this.options.sessionSelectEl ? document.querySelector(this.options.sessionSelectEl) : null;

        // State
        this.source = null;
        this.connected = false;
        this.sessionId = this.options.sessionId || null;
        this.lastPing = 0;
        this.onSend = this.options.onSend;
        this.onSendKey = this.options.onSendKey;

        // Bind methods
        this.toggle = this.toggle.bind(this);
        this.connect = this.connect.bind(this);
        this.disconnect = this.disconnect.bind(this);
        this.sendInput = this.sendInput.bind(this);
        this.sendKey = this.sendKey.bind(this);

        // Setup event listeners
        this.setupEventListeners();
    }

    setupEventListeners() {
        if (this.toggleBtn) {
            this.toggleBtn.addEventListener('click', this.toggle);
        }

        if (this.inputEl) {
            this.inputEl.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                    // Enter without Shift sends input
                    e.preventDefault();
                    this.sendInput();
                }
                // Shift+Enter allows newline in textarea
            });
        }

        // Setup session selection if available
        if (this.sessionSelectEl) {
            this.sessionSelectEl.addEventListener('change', () => {
                if (this.connected) {
                    this.disconnect();
                }
            });
        }
    }

    /**
     * Get current session ID (from options or select element)
     */
    getSessionId() {
        if (this.sessionSelectEl) {
            return this.sessionSelectEl.value;
        }
        return this.sessionId;
    }

    /**
     * Get human-friendly target label for history.
     */
    getTargetLabel() {
        if (this.sessionSelectEl && this.sessionSelectEl.selectedIndex >= 0) {
            const option = this.sessionSelectEl.options[this.sessionSelectEl.selectedIndex];
            if (option && option.textContent) {
                return option.textContent;
            }
        }
        if (this.options.targetLabel) {
            return this.options.targetLabel;
        }
        return this.sessionId || this.getSessionId() || 'unknown';
    }

    /**
     * Toggle connection state
     */
    toggle() {
        if (this.connected) {
            this.disconnect();
        } else {
            this.connect();
        }
    }

    /**
     * Connect to terminal SSE stream
     */
    connect() {
        const session = this.getSessionId();
        if (!session) {
            this.setOutput('No session selected.');
            return;
        }

        if (this.source) {
            this.source.close();
        }

        this.setOutput('Connecting to ' + session + '...');
        this.sessionId = session;
        this.lastPing = Date.now();

        const url = this.options.streamUrl + '?session=' + encodeURIComponent(session);
        this.source = new EventSource(url);
        this.connected = true;

        this.updateToggleButton(true);
        this.updateSessionInfo();

        // Frame event - terminal content
        this.source.addEventListener('frame', (e) => {
            this.setOutput(e.data);
            this.lastPing = Date.now();
        });

        // Ping event - keepalive
        this.source.addEventListener('ping', (e) => {
            this.lastPing = Date.now();
            this.updateSessionInfo();
        });

        // Open event
        this.source.addEventListener('open', () => {
            this.updateSessionInfo();
            if (this.inputArea) {
                this.inputArea.style.display = 'block';
            }
        });

        // Error event
        this.source.addEventListener('error', (e) => {
            const data = e.data || '';
            if (data.startsWith('session_ended:')) {
                this.setOutput('Session ended: ' + data.substring(14));
                this.disconnect();
            } else if (data.startsWith('transient:')) {
                console.warn('Terminal transient error:', data);
                // Don't disconnect - let SSE auto-retry
            } else if (data === 'max_errors_reached') {
                this.setOutput('Too many errors - disconnecting.');
                this.disconnect();
            } else {
                // Generic error - SSE will auto-reconnect
                console.log('Terminal SSE error, auto-reconnecting...');
                this.updateSessionInfo('reconnecting');
            }
        });
    }

    /**
     * Disconnect from terminal stream
     */
    disconnect() {
        if (this.source) {
            this.source.close();
            this.source = null;
        }
        this.connected = false;
        this.updateToggleButton(false);

        if (this.inputArea) {
            this.inputArea.style.display = 'none';
        }
    }

    /**
     * Send text input to terminal
     */
    async sendInput() {
        if (!this.inputEl || !this.sessionId) return;

        const text = this.inputEl.value;
        if (!text) return;

        try {
            const enter = this.options.sendEnter !== false;
            const res = await fetch(this.options.sendUrl, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    session: this.sessionId,
                    text: text,
                    enter: enter
                })
            });
            if (res.ok) {
                this.inputEl.value = '';
                if (typeof this.onSend === 'function') {
                    this.onSend({
                        target: this.getTargetLabel(),
                        context: 'Text: ' + text
                    });
                }
            } else {
                console.error('Failed to send input:', await res.text());
            }
        } catch (e) {
            console.error('Error sending input:', e);
        }
    }

    /**
     * Send special key to terminal
     * @param {string} key - Key name (e.g., 'C-c', 'Enter', 'Escape')
     */
    async sendKey(key) {
        if (!this.sessionId) return;

        try {
            await fetch(this.options.sendUrl, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    session: this.sessionId,
                    key: key
                })
            });
            if (typeof this.onSendKey === 'function') {
                this.onSendKey({
                    target: this.getTargetLabel(),
                    context: 'Key: ' + key
                });
            }
        } catch (e) {
            console.error('Error sending key:', e);
        }
    }

    /**
     * Set terminal output content
     * @param {string} content - Text content
     */
    setOutput(content) {
        if (this.outputEl) {
            this.outputEl.textContent = content;
            this.outputEl.scrollTop = this.outputEl.scrollHeight;
        }
    }

    /**
     * Update toggle button state
     * @param {boolean} isConnected - Current connection state
     */
    updateToggleButton(isConnected) {
        if (!this.toggleBtn) return;

        if (isConnected) {
            this.toggleBtn.textContent = 'Disconnect';
            this.toggleBtn.className = this.toggleBtn.className.replace('btn-success', 'btn-warning');
            if (!this.toggleBtn.className.includes('btn-warning')) {
                this.toggleBtn.className += ' btn-warning';
            }
        } else {
            this.toggleBtn.textContent = 'Connect';
            this.toggleBtn.className = this.toggleBtn.className.replace('btn-warning', 'btn-success');
            if (!this.toggleBtn.className.includes('btn-success')) {
                this.toggleBtn.className += ' btn-success';
            }
        }
    }

    /**
     * Update session info panel
     * @param {string} status - Optional status override
     */
    updateSessionInfo(status) {
        if (!this.sessionInfoEl) return;

        const session = this.sessionId || 'None';
        const timeSince = this.lastPing ? Math.floor((Date.now() - this.lastPing) / 1000) + 's ago' : 'never';

        let statusBadge;
        if (status === 'reconnecting') {
            statusBadge = '<span class="badge badge-yellow">Reconnecting...</span>';
        } else if (this.connected) {
            statusBadge = '<span class="badge badge-green">Connected</span>';
        } else {
            statusBadge = '<span class="badge badge-red">Disconnected</span>';
        }

        this.sessionInfoEl.innerHTML =
            '<table>' +
            '<tr><th>Session ID</th><td>' + escapeHtml(session) + '</td></tr>' +
            '<tr><th>Status</th><td>' + statusBadge + '</td></tr>' +
            '<tr><th>Last Data</th><td>' + timeSince + '</td></tr>' +
            '</table>';
    }

    /**
     * Update session selector dropdown
     * @param {Array} sessions - Array of {id, label} objects
     */
    updateSessionSelect(sessions) {
        if (!this.sessionSelectEl) return;

        const previous = this.sessionSelectEl.value;
        this.sessionSelectEl.innerHTML = '';

        if (!sessions || sessions.length === 0) {
            this.sessionSelectEl.innerHTML = '<option value="">No active sessions</option>';
            return;
        }

        sessions.forEach(s => {
            const option = document.createElement('option');
            option.value = s.id;
            option.textContent = s.label + ' (' + s.id + ')';
            this.sessionSelectEl.appendChild(option);
        });

        // Restore previous selection if still available
        if (previous && sessions.some(s => s.id === previous)) {
            this.sessionSelectEl.value = previous;
        }
    }

    /**
     * Destroy the widget and clean up
     */
    destroy() {
        this.disconnect();
        if (this.toggleBtn) {
            this.toggleBtn.removeEventListener('click', this.toggle);
        }
    }
}

// Export for use in other scripts
window.TerminalWidget = TerminalWidget;

// ================================================================
// Action History (Terminal/Major)
// ================================================================
class ActionHistory {
    constructor(options) {
        this.options = Object.assign({
            tableBodyEl: null,
            pageInfoEl: null,
            prevBtn: null,
            nextBtn: null,
            pageSizeSelectEl: null,
            storageKey: 'terminal-history',
            maxEntries: 500,
            defaultPageSize: 25,
            apiUrl: null,
        }, options);

        this.tableBody = document.querySelector(this.options.tableBodyEl);
        this.pageInfoEl = document.querySelector(this.options.pageInfoEl);
        this.prevBtn = document.querySelector(this.options.prevBtn);
        this.nextBtn = document.querySelector(this.options.nextBtn);
        this.pageSizeSelectEl = document.querySelector(this.options.pageSizeSelectEl);

        this.entries = [];
        this.page = 1;
        this.pageSize = this.options.defaultPageSize;

        this.load();
        this.bind();
        this.render();
    }

    bind() {
        if (this.prevBtn) {
            this.prevBtn.addEventListener('click', () => this.setPage(this.page - 1));
        }
        if (this.nextBtn) {
            this.nextBtn.addEventListener('click', () => this.setPage(this.page + 1));
        }
        if (this.pageSizeSelectEl) {
            this.pageSizeSelectEl.addEventListener('change', () => {
                const value = parseInt(this.pageSizeSelectEl.value, 10);
                if (!Number.isNaN(value)) {
                    this.setPageSize(value);
                }
            });
        }
    }

    load() {
        this.loadFromLocal();
        if (this.options.apiUrl) {
            this.loadFromApi();
        }
    }

    loadFromLocal() {
        try {
            const raw = localStorage.getItem(this.options.storageKey);
            if (raw) {
                const data = JSON.parse(raw);
                if (Array.isArray(data.entries)) {
                    this.entries = data.entries;
                }
                if (typeof data.pageSize === 'number' && data.pageSize > 0) {
                    this.pageSize = data.pageSize;
                }
            }
        } catch (e) {
            console.warn('Failed to load history:', e);
        }
    }

    async loadFromApi() {
        try {
            const res = await fetch(this.options.apiUrl + '?key=' + encodeURIComponent(this.options.storageKey));
            if (!res.ok) {
                throw new Error(await res.text());
            }
            const data = await res.json();
            if (Array.isArray(data.entries)) {
                this.entries = data.entries;
            }
            if (typeof data.page_size === 'number' && data.page_size > 0) {
                this.pageSize = data.page_size;
            }
            this.page = 1;
            this.saveLocal();
            this.render();
        } catch (e) {
            console.warn('Failed to load history from API:', e);
        }
    }

    saveLocal() {
        try {
            localStorage.setItem(this.options.storageKey, JSON.stringify({
                entries: this.entries,
                pageSize: this.pageSize
            }));
        } catch (e) {
            console.warn('Failed to save history:', e);
        }
    }

    async saveRemote(payload) {
        if (!this.options.apiUrl) {
            return;
        }
        try {
            await fetch(this.options.apiUrl, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });
        } catch (e) {
            console.warn('Failed to save history to API:', e);
        }
    }

    addEntry(entry) {
        const ts = Date.now();
        const time = new Date(ts).toLocaleString();
        const record = {
            timestamp: ts,
            time: time,
            target: entry.target || 'unknown',
            context: entry.context || ''
        };
        this.entries.unshift(record);
        if (this.entries.length > this.options.maxEntries) {
            this.entries = this.entries.slice(0, this.options.maxEntries);
        }
        this.page = 1;
        this.saveLocal();
        this.saveRemote({
            storage_key: this.options.storageKey,
            entry: record
        });
        this.render();
    }

    setPage(page) {
        const totalPages = this.totalPages();
        if (page < 1 || page > totalPages) {
            return;
        }
        this.page = page;
        this.render();
    }

    setPageSize(size) {
        this.pageSize = size;
        this.page = 1;
        this.saveLocal();
        this.saveRemote({
            storage_key: this.options.storageKey,
            page_size: this.pageSize
        });
        this.render();
    }

    totalPages() {
        if (this.entries.length === 0) {
            return 1;
        }
        return Math.ceil(this.entries.length / this.pageSize);
    }

    render() {
        if (!this.tableBody) {
            return;
        }
        const totalPages = this.totalPages();
        if (this.page > totalPages) {
            this.page = totalPages;
        }
        const start = (this.page - 1) * this.pageSize;
        const pageEntries = this.entries.slice(start, start + this.pageSize);

        if (this.pageSizeSelectEl) {
            this.pageSizeSelectEl.value = String(this.pageSize);
        }

        this.tableBody.innerHTML = pageEntries.map(entry => {
            const time = entry.timestamp ? new Date(entry.timestamp).toLocaleString() : (entry.time || '');
            return '<tr>' +
                '<td>' + escapeHtml(time) + '</td>' +
                '<td>' + escapeHtml(entry.target) + '</td>' +
                '<td class="history-context">' + escapeHtml(entry.context) + '</td>' +
                '</tr>';
        }).join('');

        if (this.pageInfoEl) {
            this.pageInfoEl.textContent = 'Page ' + this.page + ' / ' + totalPages + ' · ' + this.entries.length + ' entries';
        }
        if (this.prevBtn) {
            this.prevBtn.disabled = this.page <= 1;
        }
        if (this.nextBtn) {
            this.nextBtn.disabled = this.page >= totalPages;
        }
    }
}

window.ActionHistory = ActionHistory;
