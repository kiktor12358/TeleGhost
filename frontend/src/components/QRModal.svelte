<script>
    import { onMount, createEventDispatcher } from 'svelte';
    import { fade } from 'svelte/transition';
    import QRious from 'qrious';
    import { Icons } from '../Icons.js';
    import * as AppActions from '../../wailsjs/go/main/App.js';

    export let show = false;
    export let address = '';
    
    const dispatch = createEventDispatcher();
    let qrCanvas;
    let copied = false;

    $: if (show && qrCanvas && address) {
        new QRious({
            element: qrCanvas,
            value: address,
            size: 250,
            background: 'white',
            foreground: '#000000',
            level: 'H'
        });
    }

    function close() {
        dispatch('close');
    }

    async function copyToClipboard() {
        try {
            await AppActions.ClipboardSet(address);
            copied = true;
            setTimeout(() => copied = false, 2000);
            dispatch('toast', { message: 'Адрес скопирован', type: 'success' });
        } catch (err) {
            console.error('Failed to copy: ', err);
            dispatch('toast', { message: 'Ошибка копирования', type: 'error' });
        }
    }
</script>

{#if show}
<div class="modal-backdrop animate-fade-in" on:click|self={close} role="button" tabindex="0" on:keydown={(e) => e.key === 'Escape' && close()}>
    <div class="modal-content animate-slide-down">
        <div class="modal-header">
            <h3>Мой I2P адрес</h3>
            <button class="btn-icon" on:click={close}>
                <div class="icon-svg">{@html Icons.X}</div>
            </button>
        </div>
        
        <div class="modal-body qr-container">
            <div 
                class="qr-wrapper" 
                role="button" 
                tabindex="0" 
                on:click={copyToClipboard}
                on:keydown={(e) => e.key === 'Enter' && copyToClipboard()}
                title="Нажмите, чтобы скопировать"
            >
                <canvas bind:this={qrCanvas}></canvas>
                {#if copied}
                    <div class="copied-overlay" transition:fade>
                        <div class="icon-svg-lg">{@html Icons.Check}</div>
                        <span>Скопировано</span>
                    </div>
                {/if}
            </div>
            
            <p class="hint">Нажмите на QR-код, чтобы скопировать</p>
            
            <div class="address-quote">
                <blockquote>
                    {address}
                </blockquote>
            </div>
        </div>
    </div>
</div>
{/if}

<style>
    .modal-backdrop {
        position: fixed;
        top: 0;
        left: 0;
        width: 100vw;
        height: 100vh;
        background: rgba(0, 0, 0, 0.8);
        backdrop-filter: blur(10px);
        display: flex;
        align-items: center;
        justify-content: center;
        z-index: 2000;
    }

    .modal-content {
        background: var(--bg-secondary);
        border-radius: 24px;
        padding: 24px;
        width: 90%;
        max-width: 400px;
        box-shadow: 0 30px 60px rgba(0, 0, 0, 0.6);
        border: 1px solid var(--border);
        text-align: center;
    }

    .modal-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        margin-bottom: 20px;
    }

    .modal-header h3 {
        margin: 0;
        font-size: 18px;
        color: white;
    }

    .qr-container {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 16px;
    }

    .qr-wrapper {
        position: relative;
        background: white;
        padding: 12px;
        border-radius: 16px;
        cursor: pointer;
        transition: transform 0.2s;
        display: flex;
        align-items: center;
        justify-content: center;
    }

    .qr-wrapper:hover {
        transform: scale(1.02);
    }

    .copied-overlay {
        position: absolute;
        top: 0;
        left: 0;
        right: 0;
        bottom: 0;
        background: rgba(99, 102, 241, 0.9);
        border-radius: 16px;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        color: white;
        font-weight: 600;
        gap: 8px;
    }

    .hint {
        font-size: 12px;
        color: var(--text-secondary);
        margin: 0;
    }

    .address-quote {
        width: 100%;
        background: var(--bg-input);
        padding: 12px;
        border-radius: 12px;
        margin-top: 8px;
    }

    blockquote {
        margin: 0;
        font-size: 11px;
        color: var(--text-secondary);
        word-break: break-all;
        font-family: monospace;
        line-height: 1.4;
        display: -webkit-box;
        -webkit-line-clamp: 3;
        -webkit-box-orient: vertical;
        overflow: hidden;
        text-align: left;
    }

    .btn-icon {
        background: rgba(255, 255, 255, 0.05);
        border: none;
        color: white;
        width: 32px;
        height: 32px;
        border-radius: 10px;
        display: flex;
        align-items: center;
        justify-content: center;
        cursor: pointer;
    }

    .icon-svg { width: 20px; height: 20px; }
    .icon-svg-lg { width: 40px; height: 40px; }
</style>
