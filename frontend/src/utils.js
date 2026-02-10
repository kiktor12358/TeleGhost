export function getInitials(name) {
    if (!name) return 'U';
    return name.split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2);
}

export function getAvatarGradient(name) {
    if (!name) return 'linear-gradient(135deg, #6366f1, #a78bfa)';
    let hash = 0;
    for (let i = 0; i < name.length; i++) {
        hash = name.charCodeAt(i) + ((hash << 5) - hash);
    }
    const palettes = [
        ['#6366f1', '#a78bfa'], // indigo → violet
        ['#8b5cf6', '#ec4899'], // violet → pink
        ['#06b6d4', '#3b82f6'], // cyan → blue
        ['#10b981', '#06b6d4'], // emerald → cyan
        ['#f59e0b', '#ef4444'], // amber → red
        ['#ec4899', '#f97316'], // pink → orange
        ['#14b8a6', '#a78bfa'], // teal → violet
        ['#3b82f6', '#8b5cf6'], // blue → violet
        ['#f43f5e', '#fb923c'], // rose → orange
        ['#22d3ee', '#818cf8'], // cyan → indigo
        ['#a855f7', '#6366f1'], // purple → indigo
        ['#059669', '#2dd4bf'], // emerald dark → teal
    ];
    const idx = Math.abs(hash) % palettes.length;
    const angle = (Math.abs(hash >> 4) % 4) * 45 + 120; // 120-255 deg
    return `linear-gradient(${angle}deg, ${palettes[idx][0]}, ${palettes[idx][1]})`;
}

export function formatTime(timestamp) {
    if (!timestamp) return '';
    const date = new Date(timestamp);
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

export function getStatusColor(status) {
    switch (status) {
        case 'online': return '#4CAF50';
        case 'connecting': return '#FFC107';
        case 'error': return '#F44336';
        default: return '#9E9E9E';
    }
}

export function getStatusText(status) {
    switch (status) {
        case 'online': return 'В сети';
        case 'connecting': return 'Подключение...';
        case 'error': return 'Ошибка I2P';
        default: return 'Оффлайн';
    }
}

export function parseMarkdown(text) {
    if (!text) return '';

    // Simple HTML escaping
    let html = text
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');

    // Code block
    html = html.replace(/```([\s\S]*?)```/g, '<pre class="md-code-block">$1</pre>');

    // Inline code
    html = html.replace(/`([^`]+)`/g, '<code class="md-code">$1</code>');

    // Bold
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/__([^_]+)__/g, '<strong>$1</strong>');

    // Italic
    html = html.replace(/(?<!\w)\*([^*]+)\*(?!\w)/g, '<em>$1</em>');
    html = html.replace(/(?<!\w)_([^_]+)_(?!\w)/g, '<em>$1</em>');

    // Strikethrough
    html = html.replace(/~~([^~]+)~~/g, '<del>$1</del>');

    // Links (simple)
    html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>');

    // Line breaks
    return html.replace(/\n/g, '<br>');
}
