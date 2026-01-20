// Docs page logic: file tree + markdown rendering
(function () {
    const treeContainer = document.getElementById('docs-tree');
    if (!treeContainer) return;

    const filterInput = document.getElementById('docs-filter');
    const countEl = document.getElementById('docs-count');
    const renderEl = document.getElementById('doc-render');
    const pathEl = document.getElementById('doc-path');
    const refreshBtn = document.getElementById('docs-refresh');

    const safeEscape = typeof escapeHtml === 'function'
        ? escapeHtml
        : (text) => String(text).replace(/[&<>"']/g, (c) => ({
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            '"': '&quot;',
            "'": '&#39;',
        }[c]));

    let tree = null;
    let allFiles = [];
    let nodes = new Map();
    let selectedPath = '';

    if (window.marked) {
        window.marked.setOptions({ gfm: true, breaks: true });
    }

    function setStatus(message, isError) {
        renderEl.innerHTML = '<p class="' + (isError ? 'text-danger' : 'text-muted') + '">' +
            safeEscape(message) + '</p>';
    }

    function updateCount(total, shown) {
        if (!countEl) return;
        const suffix = total === shown ? '' : ' (filtered)';
        countEl.textContent = shown + ' files' + suffix;
    }

    function decodeHash() {
        if (!window.location.hash) return '';
        try {
            return decodeURIComponent(window.location.hash.slice(1));
        } catch (err) {
            return '';
        }
    }

    function setHash(path) {
        const next = '#' + encodeURIComponent(path);
        if (window.location.hash !== next) {
            history.replaceState(null, '', next);
        }
    }

    function buildTree(paths) {
        treeContainer.innerHTML = '';
        nodes = new Map();

        if (!window.VanillaTree) {
            setStatus('Docs tree library failed to load.', true);
            return;
        }

        tree = new window.VanillaTree(treeContainer, {
            placeholder: 'No markdown files found',
        });

        const entries = [];
        paths.forEach((path) => {
            const parts = path.split('/');
            let parent = null;
            let current = '';
            parts.forEach((part, index) => {
                current = current ? current + '/' + part : part;
                const isFile = index === parts.length - 1;
                if (!nodes.has(current)) {
                    const node = {
                        id: current,
                        label: part,
                        parent: parent,
                        type: isFile ? 'file' : 'dir',
                        opened: parent === null,
                    };
                    nodes.set(current, node);
                    entries.push(node);
                }
                parent = current;
            });
        });

        entries.sort((a, b) => {
            const depthDiff = a.id.split('/').length - b.id.split('/').length;
            if (depthDiff !== 0) return depthDiff;
            if (a.type !== b.type) return a.type === 'dir' ? -1 : 1;
            return a.label.localeCompare(b.label);
        });

        entries.forEach((node) => {
            tree.add({
                id: node.id,
                label: safeEscape(node.label),
                parent: node.parent || undefined,
                opened: node.opened,
            });
        });

        if (selectedPath && nodes.has(selectedPath)) {
            tree.select(selectedPath);
        }
    }

    async function loadTree() {
        try {
            setStatus('Loading docs tree...', false);
            const res = await fetch('/api/docs/tree');
            if (!res.ok) {
                throw new Error(await res.text());
            }
            const data = await res.json();
            allFiles = Array.isArray(data.files) ? data.files : [];
            applyFilter();
            const fromHash = decodeHash();
            if (fromHash && nodes.has(fromHash)) {
                loadDoc(fromHash);
            } else if (allFiles.length > 0) {
                const first = allFiles[0];
                loadDoc(first);
                if (tree && nodes.has(first)) {
                    tree.select(first);
                }
            } else {
                setStatus('No markdown files found.', false);
            }
        } catch (err) {
            setStatus('Failed to load docs tree: ' + err.message, true);
        }
    }

    function applyFilter() {
        const query = filterInput ? filterInput.value.trim().toLowerCase() : '';
        const filtered = query
            ? allFiles.filter((path) => path.toLowerCase().includes(query))
            : allFiles.slice();
        buildTree(filtered);
        updateCount(allFiles.length, filtered.length);
    }

    async function loadDoc(path) {
        selectedPath = path;
        if (pathEl) {
            pathEl.textContent = path;
        }
        setHash(path);
        renderEl.innerHTML = '<p class="text-muted">Loading...</p>';
        try {
            const res = await fetch('/api/docs/file?path=' + encodeURIComponent(path));
            if (!res.ok) {
                throw new Error(await res.text());
            }
            const data = await res.json();
            const raw = data.content || '';
            if (!window.marked) {
                renderEl.textContent = raw;
                return;
            }
            const html = window.marked.parse(raw);
            if (window.DOMPurify) {
                renderEl.innerHTML = window.DOMPurify.sanitize(html);
            } else {
                renderEl.innerHTML = html;
            }
        } catch (err) {
            setStatus('Failed to load doc: ' + err.message, true);
        }
    }

    if (filterInput) {
        filterInput.addEventListener('input', () => applyFilter());
    }
    if (refreshBtn) {
        refreshBtn.addEventListener('click', () => loadTree());
    }
    treeContainer.addEventListener('vtree-select', (event) => {
        const id = event?.detail?.id;
        const node = nodes.get(id);
        if (!node || node.type !== 'file') {
            return;
        }
        loadDoc(id);
    });
    window.addEventListener('hashchange', () => {
        const fromHash = decodeHash();
        if (fromHash && nodes.has(fromHash)) {
            loadDoc(fromHash);
        }
    });

    loadTree();
})();
