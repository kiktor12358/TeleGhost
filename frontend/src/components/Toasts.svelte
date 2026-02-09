<script>
    import { toasts } from '../stores.js';
</script>

<div class="toast-container">
    {#each $toasts as toast (toast.id)}
        <div class="toast toast-{toast.type}">
            {toast.message}
        </div>
    {/each}
</div>

<style>
    .toast-container {
        position: fixed;
        top: 20px;
        right: 20px;
        z-index: 10000;
        display: flex;
        flex-direction: column;
        gap: 10px;
        pointer-events: none;
    }

    .toast {
        padding: 12px 20px;
        border-radius: 8px;
        color: white;
        font-size: 14px;
        box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
        animation: toast-enter 0.3s ease-out, toast-exit 0.3s ease-in 2.7s forwards;
        pointer-events: auto;
        max-width: 320px;
    }

    .toast-success {
        background: linear-gradient(135deg, #4CAF50, #2E7D32);
    }

    .toast-error {
        background: linear-gradient(135deg, #F44336, #C62828);
    }

    .toast-info {
        background: linear-gradient(135deg, var(--accent, #6c5ce7), #5a4fcf);
    }

    @keyframes toast-enter {
        from {
            transform: translateX(100px);
            opacity: 0;
        }
        to {
            transform: translateX(0);
            opacity: 1;
        }
    }

    @keyframes toast-exit {
        from {
            transform: translateX(0);
            opacity: 1;
        }
        to {
            transform: translateX(100px);
            opacity: 0;
        }
    }
</style>
