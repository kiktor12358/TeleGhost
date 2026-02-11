<script>
    import { Icons } from '../Icons.js';
    import { getInitials, formatTime, parseMarkdown, getAvatarGradient } from '../utils.js';
    import { fade, fly } from 'svelte/transition';
    import { onMount, tick } from 'svelte';

    export let selectedContact;
    export let messages = [];
    export let newMessage;
    export let selectedFiles = [];
    export let filePreviews = {};
    export let editingMessageId = null;
    export let editMessageContent = '';
    export let isCompressed = false;
    export let replyingTo = null;
    export let isMobile = false;
    export let onBack = null;
    export let onCancelReply;

    export let onSendMessage;
    export let onKeyPress;
    export let onPaste;
    export let onSelectFiles;
    export let onRemoveFile;
    export let onShowMessageMenu;
    export let onAcceptTransfer;
    export let onDeclineTransfer;
    export let onOpenContactProfile;
    export let onSaveEditMessage;
    export let onCancelEdit;
    export let onOpenFile;
    export let onSaveFile;
    export let onPreviewImage;
    export let startLoadingImage; // Fix: Add missing prop

    let textarea;
    let touchStartX = 0;
    let touchMoveX = 0;
    let swipedMsgId = null;
    let swipeThreshold = 60;
    
    // Image loading state
    let imagesLoading = false;
    let pendingImages = 0;
    let loadedImages = 0;
    let chatReady = false;
    let initialScrollDone = false;
    let currentContactId = null;


    function handleTouchStart(e, msgId) {
        if (msgId === swipedMsgId) return;
        touchStartX = e.touches[0].clientX;
        swipedMsgId = msgId;
    }

    function handleTouchMove(e) {
        if (!swipedMsgId) return;
        const currentX = e.touches[0].clientX;
        const diff = touchStartX - currentX;
        if (diff > 0) { // Swiping left
            touchMoveX = Math.min(diff, 100); // Limit visual swipe
        }
    }

    function handleTouchEnd(msg) {
        if (swipedMsgId === msg.ID && touchMoveX >= swipeThreshold) {
            handleReply(msg);
            if (navigator.vibrate) navigator.vibrate(50);
        }
        touchMoveX = 0;
        swipedMsgId = null;
    }

    function handleReply(msg) {
        replyingTo = msg;
        if (textarea) textarea.focus();
    }

    function handleDoubleClick(msg) {
        if (!isMobile) {
            handleReply(msg);
        }
    }

    let showScrollButton = false;
    let resizeObserver;
    let containerRef;

    function handleScroll(e) {
        const container = e.target;
        const distanceToBottom = container.scrollHeight - container.scrollTop - container.clientHeight;
        showScrollButton = distanceToBottom > 50;
    }

    function scrollToBottom(force = false) {
        // Guard against unnecessary scrolls
        if (!processScroll(force)) return;

        tick().then(() => {
            requestAnimationFrame(() => {
                if (containerRef) {
                     // If force is true, we scroll to bottom no matter what
                    if (force) {
                        containerRef.scrollTop = containerRef.scrollHeight;
                        return;
                    }
                    
                    // If content grew and we were ALREADY at bottom (or close), stay at bottom
                    const distanceToBottom = containerRef.scrollHeight - containerRef.scrollTop - containerRef.clientHeight;
                    // Relaxed threshold
                    if (distanceToBottom < 100) {
                         containerRef.scrollTop = containerRef.scrollHeight;
                    }
                }
            });
        });
    }

    function processScroll(force) {
        if (!containerRef) return false;
        if (!force && (!messages || messages.length === 0)) return false;
        return true;
    }

    onMount(() => {
        if (containerRef) {
            // We only need to observe to SHOW/HIDE the button, NOT to force scroll
            resizeObserver = new ResizeObserver(() => {
                if (!containerRef) return;
                const distanceToBottom = containerRef.scrollHeight - containerRef.scrollTop - containerRef.clientHeight;
                showScrollButton = distanceToBottom > 50;
            });
            resizeObserver.observe(containerRef);
            
            return () => {
                resizeObserver.disconnect();
            };
        }
    });

    // Handle Contact Change & Initialize Loading
    $: if (selectedContact && selectedContact.ID !== currentContactId) {
        currentContactId = selectedContact.ID;
        // Reset state for new chat
        chatReady = false; 
        initialScrollDone = false;
        imagesLoading = true;
        pendingImages = 0;
        loadedImages = 0;
    }

    // Handle Messages Update & Image Counting
    $: if (messages && !chatReady && currentContactId) {
        // Count images ONLY in the initial load phase
        let imgCount = 0;
        // Check only last 20 messages for performance, deeper history images shouldn't affect initial scroll much
        const recentMessages = messages.slice(-20); 
        
        recentMessages.forEach(msg => {
            if (msg.Attachments) {
                msg.Attachments.forEach(att => {
                    if (att.MimeType && att.MimeType.startsWith('image/')) {
                        imgCount++;
                    }
                });
            }
        });

        pendingImages = imgCount;
        
        if (pendingImages === 0) {
            // No images to wait for (or messages empty), show immediately
            finishLoading();
        } else {
            // Set a fallback timeout in case images fail to load
            setTimeout(() => {
                if (!chatReady) finishLoading();
            }, 1500); // 1.5s max wait
        }
    }

    function onImageLoad() {
        if (chatReady) return;
        loadedImages++;
        if (loadedImages >= pendingImages) {
            finishLoading();
        }
    }

    // Auto-scroll on new messages
    $: if (messages && messages.length > 0 && chatReady) {
        scrollToBottom();
    }

    function finishLoading() {
        if (chatReady) return;
        
        // Scroll while invisible FIRST
        tick().then(() => {
             scrollToBottom(true);
             
             // Slight delay to ensure layout paints and scroll applies BEFORE we show it
             setTimeout(() => {
                 scrollToBottom(true); // Scroll again to be sure
                 chatReady = true;
                 imagesLoading = false;
             }, 250);
        });
    }

</script>

<div class="chat-area animate-fade-in" class:mobile={isMobile}>
    <div class="chat-header">
        {#if isMobile && onBack}
            <button class="btn-back" on:click={onBack} style="margin-right: 8px;">
                <div class="icon-svg">{@html Icons.ArrowLeft || '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"></polyline></svg>'}</div>
            </button>
        {/if}
        <div 
            class="chat-contact-info" 
            role="button"
            tabindex="0"
            on:click={onOpenContactProfile} 
            on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && onOpenContactProfile()}
            style="cursor: pointer;"
        >
            <div class="chat-avatar" style="background: {getAvatarGradient(selectedContact?.Nickname)};">
                {#if selectedContact?.Avatar}
                    <img src={selectedContact.Avatar} alt="av"/>
                {:else}
                    <div class="avatar-placeholder">{getInitials(selectedContact?.Nickname)}</div>
                {/if}
            </div>
            <div>
                <div class="chat-name">{selectedContact?.Nickname || 'Unknown'}</div>
                <div class="chat-status">
                    <span class="status-dot" style="background: {(messages || []).some(m => !m.IsOutgoing && m.SenderID === selectedContact?.PublicKey && (Date.now() - m.Timestamp < 300000)) ? '#4CAF50' : '#9E9E9E'};"></span>
                    <span class="status-text">
                        {(messages || []).some(m => !m.IsOutgoing && m.SenderID === selectedContact?.PublicKey && (Date.now() - m.Timestamp < 300000)) ? 'В сети' : 'Оффлайн'}
                    </span>
                </div>
            </div>
        </div>
    </div>
    
    <div class="messages-container messages-scroll-area" bind:this={containerRef} on:scroll={handleScroll} style="opacity: {chatReady ? 1 : 0}; transition: opacity 0.2s;">
        {#each messages as msg (msg.ID)}
            <div 
                class="message animate-message" 
                id="msg-{msg.ID}" 
                class:outgoing={msg.IsOutgoing}
                on:touchstart={(e) => isMobile && handleTouchStart(e, msg.ID)}
                on:touchmove={(e) => isMobile && handleTouchMove(e)}
                on:touchend={() => isMobile && handleTouchEnd(msg)}
                on:dblclick={() => handleDoubleClick(msg)}
                on:click={(e) => {
                    if (isMobile) {
                        onShowMessageMenu(e, msg);
                    }
                }}
            >
                <div 
                    class="message-bubble-wrapper" 
                    style="transform: translateX(-{swipedMsgId === msg.ID ? touchMoveX : 0}px); transition: {swipedMsgId === msg.ID ? 'none' : 'transform 0.3s cubic-bezier(0.18, 0.89, 0.32, 1.28)'};"
                >
                    <div class="message-bubble" class:outgoing={msg.IsOutgoing} 
                         on:contextmenu|preventDefault={(e) => onShowMessageMenu(e, msg)}
                    >
                        {#if msg.ReplyPreview}
                            <div 
                                class="reply-preview-bubble" 
                                role="button" 
                                tabindex="0"
                                on:click|stopPropagation={() => {
                                    const target = document.getElementById(`msg-${msg.ReplyToID}`);
                                    if (target) target.scrollIntoView({ behavior: 'smooth', block: 'center' });
                                }}
                            >
                                <div class="reply-author">{msg.ReplyPreview.author_name || msg.ReplyPreview.AuthorName}</div>
                                <div class="reply-content-preview">{msg.ReplyPreview.content || msg.ReplyPreview.Content}</div>
                            </div>
                        {/if}
                        {#if msg.Attachments && msg.Attachments.length > 0}
                            <div class="message-images" style="grid-template-columns: {msg.Attachments.length === 1 ? '1fr' : 'repeat(2, 1fr)'}">
                                {#each msg.Attachments as att}
                                    {#if att.MimeType && att.MimeType.startsWith('image/')}
                                        <img 
                                            use:startLoadingImage={att.LocalPath} 
                                            alt="attachment" 
                                            class="msg-img" 
                                            style="height: {msg.Attachments.length === 1 ? 'auto' : '120px'}; min-height: 100px;" 
                                            role="button"
                                            tabindex="0"
                                            on:click|stopPropagation={() => onPreviewImage(att.LocalPath)}
                                            on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && onPreviewImage(att.LocalPath)}
                                            on:load={onImageLoad}
                                        />
                                    {:else}
                                        <div class="file-attachment-container">
                                            <div 
                                                class="file-attachment-card" 
                                                role="button"
                                                tabindex="0"
                                                on:click|stopPropagation={() => onOpenFile(att.LocalPath)}
                                                on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && onOpenFile(att.LocalPath)}
                                            >
                                                <div class="file-icon">📄</div>
                                                <div class="file-info">
                                                    <div class="file-name">{att.Filename || 'File'}</div>
                                                    <div class="file-size">{att.Size ? (att.Size / 1024).toFixed(1) + ' KB' : ''}</div>
                                                </div>
                                            </div>
                                            <button class="btn-file-save" on:click|stopPropagation={() => onSaveFile(att.LocalPath, att.Filename)} title="Сохранить в другое место">
                                                <div class="icon-svg-xs">{@html Icons.Download || '⬇️'}</div>
                                            </button>
                                        </div>
                                    {/if}
                                {/each}
                            </div>
                        {/if}

                        {#if editingMessageId === msg.ID}
                            <div class="message-edit-container">
                                <textarea class="message-edit-input" bind:value={editMessageContent} on:keydown={(e) => e.key === 'Escape' && onCancelEdit()}></textarea>
                                <div class="message-edit-actions">
                                    <button class="btn-sm btn-primary" on:click={onSaveEditMessage}>✓</button>
                                    <button class="btn-sm btn-secondary" on:click={onCancelEdit}>✕</button>
                                </div>
                            </div>
                        {:else if msg.ContentType === 'file_offer'}
                            <div class="file-offer-card">
                                <div class="file-icon-large">📁</div>
                                <div class="file-info">
                                    <div class="file-title">Файлов: {msg.FileCount}</div>
                                    <div class="file-size">{(msg.TotalSize / (1024*1024)).toFixed(2)} MB</div>
                                </div>
                            </div>
                            <div class="file-actions">
                                {#if !msg.IsOutgoing}
                                    <button class="btn-small btn-success" on:click|stopPropagation={() => onAcceptTransfer(msg)}>Принять</button>
                                    <button class="btn-small btn-danger" on:click|stopPropagation={() => onDeclineTransfer(msg)}>Отклонить</button>
                                {/if}
                            </div>
                        {:else}
                            <div class="message-content">{@html parseMarkdown(msg.Content)}</div>
                        {/if}

                        <div class="message-meta">
                            <span class="message-time">{formatTime(msg.Timestamp)}</span>
                            {#if msg.IsOutgoing}
                                <span class="message-status"><div class="icon-svg-sm" style="display:inline-block; width:12px; height:12px;">{@html msg.Status === 'sending' ? Icons.Clock : Icons.Check}</div></span>
                            {/if}
                        </div>
                    </div>
                </div>
                {#if isMobile && swipedMsgId === msg.ID}
                    <div class="swipe-reply-icon" style="opacity: {Math.min(touchMoveX / swipeThreshold, 1)}; transform: scale({Math.min(touchMoveX / swipeThreshold, 1)})">
                        <div class="icon-svg-sm">{@html Icons.Reply || '↩️'}</div>
                    </div>
                {/if}
            </div>
        {/each}
        
        {#if showScrollButton}
            <button class="btn-scroll-bottom" transition:fly={{ y: 20, duration: 200 }} on:click={() => scrollToBottom(true)}>
                <div class="icon-svg">{@html Icons.ArrowDown}</div>
            </button> 
        {/if}
    </div>

    <!-- Loading Spinner Overlay -->
    {#if !chatReady}
        <div class="loading-overlay" transition:fade={{duration: 200}}>
            <div class="spinner"></div>
        </div>
    {/if}

    <div class="input-area-wrapper">
        {#if replyingTo}
            <div class="replying-to-bar" transition:fade={{duration: 150}}>
                <div class="reply-line"></div>
                <div class="reply-info">
                    <div class="reply-author-name">Ответ для {replyingTo.IsOutgoing ? 'Меня' : ((selectedContact.Nickname?.length > 50 ? selectedContact.Nickname.substring(0, 47) + '...' : selectedContact.Nickname) || 'Unknown')}</div>
                    <div class="reply-text-preview">{replyingTo.Content?.length > 100 ? replyingTo.Content.substring(0, 97) + '...' : (replyingTo.Content || (replyingTo.ContentType === 'mixed' ? '📷 Фото' : '📎 Файл'))}</div>
                </div>
                <button class="btn-cancel-reply" on:click={() => { replyingTo = null; if (onCancelReply) onCancelReply(); }}>
                    <div class="icon-svg-xs">{@html Icons.Plus}</div>
                </button>
            </div>
        {/if}

        <div class="attachment-preview-wrapper" style="display: {selectedFiles.length > 0 ? 'block' : 'none'}">
            <div class="attachment-preview-container">
                <div class="attachment-preview" style="max-width: calc(100% - 60px);">
                    {#each selectedFiles as file, i}
                        <div class="preview-item">
                            {#if filePreviews[file]}
                                <img src={`data:image/png;base64,${filePreviews[file]}`} alt="preview" />
                            {:else}
                                <div class="file-icon-preview">📄</div>
                            {/if}
                            <button class="btn-remove-att" on:click={() => onRemoveFile(i)}>X</button>
                        </div>
                    {/each}
                </div>
                <div class="preview-actions">
                    <button class="btn-toggle-comp {isCompressed ? 'active' : ''}" on:click={() => isCompressed = !isCompressed} title={isCompressed ? 'Сжимать изображения (быстро)' : 'Отправлять оригинал (требует принятия)'}>
                        <div class="icon-svg" style="opacity: {isCompressed ? 1 : 0.5}">{@html isCompressed ? Icons.Image : Icons.File}</div>
                    </button>
                </div>
            </div>
        </div>
        
        <div class="input-area">
            <button class="btn-icon" on:click={onSelectFiles} title="Прикрепить файл">
                <div class="icon-svg">{@html Icons.Paperclip}</div>
            </button>
            <div style="flex: 1; position: relative;">
                <textarea
                    bind:this={textarea}
                    class="message-input"
                    placeholder="Сообщение..."
                    bind:value={newMessage}
                    on:keypress={onKeyPress}
                    on:paste={onPaste}
                    rows="1"
                    style="width: 100%;"
                ></textarea>
            </div>
            <button class="btn-send" on:click={onSendMessage} disabled={!(newMessage || "").trim() && (selectedFiles || []).length === 0}>
                <div class="icon-svg">{@html Icons.Send}</div>
            </button>
        </div>
    </div>
</div>

<style>
    .chat-area { flex: 1; display: flex; flex-direction: column; background: var(--bg-primary, #0c0c14); overflow: hidden; overflow-x: hidden; position: relative; overscroll-behavior: contain; width: 100%; }
    .chat-header { height: 64px; padding: 0 16px; display: flex; align-items: center; justify-content: flex-start; gap: 4px; background: var(--bg-secondary, #1e1e2e); border-bottom: 1px solid var(--border); z-index: 10; flex-shrink: 0; }
    .chat-contact-info { display: flex; align-items: center; gap: 12px; }
    .chat-avatar { width: 40px; height: 40px; border-radius: 50%; display: flex; align-items: center; justify-content: center; color: white; font-weight: bold; overflow: hidden; background: var(--accent); }
    .chat-avatar img { width: 100%; height: 100%; object-fit: cover; }
    .avatar-placeholder { font-size: 14px; }
    .chat-name { font-weight: 600; font-size: 16px; color: white; }
    .chat-status { display: flex; align-items: center; gap: 6px; }
    .status-dot { width: 8px; height: 8px; border-radius: 50%; }
    .status-text { font-size: 12px; color: var(--text-secondary); }

    .messages-container { flex: 1; overflow-y: auto; padding: 20px; display: flex; flex-direction: column; gap: 4px; }
    .message { display: flex; margin-bottom: 2px; position: relative; align-items: center; }
    .message.outgoing { justify-content: flex-end; }
    
    .message-bubble-wrapper {
        max-width: 85%;
        display: flex;
        flex-direction: column;
        z-index: 2;
    }
    .message.outgoing .message-bubble-wrapper { align-items: flex-end; }

    .message-bubble { 
        padding: 8px 12px; border-radius: 18px; background: var(--bg-secondary, #1e1e2e); color: var(--text-primary); position: relative; box-shadow: 0 2px 5px rgba(0,0,0,0.1); width: fit-content;
    }
    .message.outgoing .message-bubble { background: var(--accent, #6366f1); color: white; border-bottom-right-radius: 4px; }
    .message:not(.outgoing) .message-bubble { border-bottom-left-radius: 4px; }

    .swipe-reply-icon {
        position: absolute;
        right: -40px;
        width: 32px;
        height: 32px;
        background: rgba(255, 255, 255, 0.1);
        border-radius: 50%;
        display: flex;
        align-items: center;
        justify-content: center;
        color: var(--accent);
        pointer-events: none;
    }

    .message-images { display: grid; gap: 4px; margin-bottom: 6px; border-radius: 8px; overflow: hidden; }
    .msg-img { width: 100%; object-fit: cover; cursor: pointer; background: rgba(0,0,0,0.2); }

    .message-content { 
        overflow-wrap: anywhere; 
        word-break: break-word; 
        max-width: 100%;
    }
    .message-meta { display: flex; align-items: center; gap: 6px; margin-top: 4px; justify-content: flex-end; opacity: 0.7; font-size: 10px; }
    .message-time { white-space: nowrap; }

    .input-area-wrapper { padding: 10px 20px 20px; background: var(--bg-primary); position: sticky; bottom: 0; z-index: 50; border-top: 1px solid var(--border); }
    .input-area { display: flex; align-items: center; gap: 10px; background: var(--bg-secondary); padding: 8px 12px; border-radius: 24px; }
    .message-input { flex: 1; background: transparent; border: none; color: white; outline: none; resize: none; font-size: 15px; max-height: 150px; padding: 8px 0; }

    .btn-icon { background: transparent; border: none; color: var(--text-secondary); cursor: pointer; padding: 8px; border-radius: 50%; display: flex; align-items: center; justify-content: center; transition: background 0.2s; }
    .btn-icon:hover { background: rgba(255,255,255,0.1); color: white; }
    .btn-icon.active { color: var(--accent); }

    .btn-send { width: 40px; height: 40px; border-radius: 50%; background: var(--accent); color: white; border: none; cursor: pointer; display: flex; align-items: center; justify-content: center; transition: transform 0.2s; flex-shrink: 0; }
    .btn-send:hover { transform: scale(1.05); }
    .btn-send:disabled { opacity: 0.5; cursor: not-allowed; }

    .attachment-preview-container { display: flex; align-items: center; justify-content: space-between; background: var(--bg-secondary); border-radius: 12px; margin-bottom: 10px; padding: 10px; }
    .attachment-preview { display: flex; gap: 10px; overflow-x: auto; scrollbar-width: none; }
    .attachment-preview::-webkit-scrollbar { display: none; }
    .preview-item { position: relative; width: 60px; height: 60px; border-radius: 8px; overflow: hidden; flex-shrink: 0; background: rgba(0,0,0,0.2); }
    .preview-item img { width: 100%; height: 100%; object-fit: cover; }
    .btn-remove-att { position: absolute; top: 2px; right: 2px; background: rgba(0,0,0,0.5); color: white; border: none; border-radius: 50%; width: 18px; height: 18px; font-size: 10px; cursor: pointer; }
    
    .preview-actions { padding-left: 10px; border-left: 1px solid var(--border); }
    .btn-toggle-comp { background: transparent; border: none; color: var(--text-secondary); cursor: pointer; padding: 8px; border-radius: 8px; display: flex; align-items: center; justify-content: center; transition: all 0.2s; }
    .btn-toggle-comp.active { color: var(--accent); background: rgba(99, 102, 241, 0.1); }
    .btn-toggle-comp:hover { background: rgba(255,255,255,0.1); }

    .file-attachment-container { display: flex; align-items: center; gap: 8px; background: rgba(0,0,0,0.1); border-radius: 12px; padding: 4px; }
    .btn-file-save { background: transparent; border: none; color: white; opacity: 0.6; cursor: pointer; padding: 8px; border-radius: 50%; transition: opacity 0.2s, background 0.2s; }
    .btn-file-save:hover { opacity: 1; background: rgba(255,255,255,0.1); }
    .icon-svg-xs { width: 14px; height: 14px; }
    .icon-svg-xs :global(svg) { width: 100%; height: 100%; }

    .icon-svg { width: 24px; height: 24px; display: flex; align-items: center; justify-content: center; }
    .icon-svg :global(svg) { width: 100%; height: 100%; }
    .icon-svg-sm { width: 18px; height: 18px; display: flex; align-items: center; justify-content: center; }
    .icon-svg-sm :global(svg) { width: 100%; height: 100%; }

    /* Back button */
    .btn-back {
        background: transparent;
        border: none;
        color: var(--text-secondary);
        cursor: pointer;
        padding: 8px;
        border-radius: 8px;
        display: flex;
        align-items: center;
        justify-content: center;
        transition: background 0.2s;
        flex-shrink: 0;
    }
    .btn-back:hover { background: rgba(255,255,255,0.1); color: white; }

    /* Mobile-specific overrides */
    .chat-area.mobile .input-area-wrapper { padding: 10px 12px calc(10px + env(safe-area-inset-bottom, 5px)); background: var(--bg-primary); }
    .chat-area.mobile .input-area { padding: 6px 10px; border-radius: 20px; }
    .chat-area.mobile .chat-header { height: 56px; padding: 0 12px; }
    /* Fix header alignment for mobile */
    .chat-area.mobile .chat-contact-info { margin-left: auto; flex-direction: row-reverse; }
    .chat-area.mobile .chat-status { flex-direction: row-reverse; }
    .chat-area.mobile .chat-name { text-align: right; }

    /* Reply Styling */
    .reply-preview-bubble {
        background: rgba(0, 0, 0, 0.05);
        border-left: 3px solid var(--accent);
        padding: 4px 8px;
        margin-bottom: 6px;
        border-radius: 4px;
        font-size: 13px;
        cursor: pointer;
        max-width: 100%;
        overflow: hidden;
    }
    .message.outgoing .reply-preview-bubble { background: rgba(255, 255, 255, 0.15); border-left-color: white; }
    .reply-author { font-weight: 600; color: var(--accent); margin-bottom: 2px; font-size: 12px; }
    .message.outgoing .reply-author { color: white; }
    .reply-content-preview { 
        color: var(--text-secondary); 
        white-space: nowrap; 
        overflow: hidden; 
        text-overflow: ellipsis; 
        font-size: 11px; 
        max-width: 100%; 
        display: -webkit-box;
        -webkit-line-clamp: 1;
        -webkit-box-orient: vertical;
        white-space: normal; /* Override nowrap for line-clamping */
    }
    .message.outgoing .reply-content-preview { color: rgba(255, 255, 255, 0.8); }

    .replying-to-bar {
        display: flex;
        align-items: center;
        gap: 12px;
        background: var(--bg-secondary);
        padding: 8px 16px;
        border-top: 1px solid var(--border);
        margin-bottom: 4px;
        border-radius: 12px 12px 0 0;
    }
    .reply-line { width: 3px; height: 32px; background: var(--accent); border-radius: 2px; }
    .reply-info { flex: 1; overflow: hidden; }
    .reply-author-name { font-size: 12px; font-weight: 600; color: var(--accent); margin-bottom: 2px; }
    .reply-text-preview { font-size: 11px; color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%; }
    .btn-cancel-reply { background: transparent; border: none; color: var(--text-secondary); cursor: pointer; padding: 4px; border-radius: 50%; display: flex; transform: rotate(45deg); }
    .btn-cancel-reply:hover { color: #ff6b6b; }

    .btn-scroll-bottom {
        position: fixed;
        bottom: 90px;
        right: 20px;
        width: 44px;
        height: 44px;
        border-radius: 50%;
        background: var(--bg-secondary);
        border: 1px solid var(--border);
        box-shadow: 0 4px 12px rgba(0,0,0,0.3);
        color: var(--accent);
        cursor: pointer;
        display: flex;
        align-items: center;
        justify-content: center;
        z-index: 90;
        transition: transform 0.2s, background 0.2s;
    }
    .btn-scroll-bottom:hover {
        transform: translateY(-2px);
        background: rgba(255, 255, 255, 0.1);
    }
    
    .loading-overlay {
        position: absolute;
        top: 64px;
        left: 0;
        right: 0;
        bottom: 90px;
        background: var(--bg-primary);
        display: flex;
        align-items: center;
        justify-content: center;
        z-index: 20;
    }
    .spinner {
        width: 40px;
        height: 40px;
        border: 3px solid rgba(255,255,255,0.1);
        border-top-color: var(--accent);
        border-radius: 50%;
        animation: spin 1s linear infinite;
    }
    @keyframes spin {
        to { transform: rotate(360deg); }
    }
</style>
