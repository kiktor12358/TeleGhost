package com.teleghost.app

import android.app.*
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import android.util.Log
import androidx.core.app.NotificationCompat

/**
 * TeleGhostService — Foreground Service для работы I2P в фоне.
 *
 * Ключевые моменты:
 * - Foreground Service гарантирует, что Android не убьёт процесс I2P.
 * - Постоянное уведомление в шторке информирует пользователя.
 * - WakeLock предотвращает засыпание CPU (критично для I2P).
 * - При нажатии на уведомление — открывается MainActivity.
 * - Кнопка "Остановить" в уведомлении завершает service.
 *
 * Жизненный цикл:
 *   MainActivity → startForegroundService() → onStartCommand() → Mobile.start()
 *   Уведомление "Остановить" → stopSelf() → Mobile.stop()
 */
class TeleGhostService : Service() {

    companion object {
        private const val TAG = "TeleGhostService"
        private const val CHANNEL_ID = "teleghost_i2p_channel"
        private const val NOTIFICATION_ID = 1337
        private const val WAKELOCK_TAG = "TeleGhost::I2PService"
    }

    private var wakeLock: PowerManager.WakeLock? = null
    private var isGoServerRunning = false

    override fun onCreate() {
        super.onCreate()
        Log.i(TAG, "Service created")
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        Log.i(TAG, "Service started")

        // Обработка action "STOP" из уведомления
        if (intent?.action == "STOP_SERVICE") {
            Log.i(TAG, "Stop action received")
            stopGoServer()
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
            return START_NOT_STICKY
        }

        // Показываем уведомление НЕМЕДЛЕННО (требование Android для Foreground Service)
        val notification = buildNotification()
        startForeground(NOTIFICATION_ID, notification)

        // Захватываем WakeLock
        acquireWakeLock()

        // Стартуем Go HTTP сервер в фоне
        startGoServer()

        // START_STICKY = системa перезапустит service если его убьют
        return START_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        Log.i(TAG, "Service destroyed")
        stopGoServer()
        releaseWakeLock()
        super.onDestroy()
    }

    // ─── Go Server Management ─────────────────────────────────────────────

    private fun startGoServer() {
        if (isGoServerRunning) {
            Log.w(TAG, "Go server already running")
            return
        }

        Thread {
            try {
                val dataDir = filesDir.absolutePath
                Log.i(TAG, "Starting Go server with dataDir=$dataDir")

                // Вызов gomobile-сгенерированного метода
                // Mobile — автоматически созданный класс из пакета mobile
                mobile.Mobile.start(dataDir)

                isGoServerRunning = true
                Log.i(TAG, "Go server started successfully")
            } catch (t: Throwable) {
                Log.e(TAG, "CRITICAL ERROR: Failed to start Go server", t)
                // Можно отправить уведомление пользователю
            }
        }.start()
    }

    private fun stopGoServer() {
        if (!isGoServerRunning) return

        try {
            mobile.Mobile.stop()
            isGoServerRunning = false
            Log.i(TAG, "Go server stopped")
        } catch (e: Exception) {
            Log.e(TAG, "Error stopping Go server", e)
        }
    }

    // ─── Notification ─────────────────────────────────────────────────────

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                getString(R.string.notification_channel_name),
                NotificationManager.IMPORTANCE_LOW // LOW = silent but visible in shade
            ).apply {
                description = getString(R.string.notification_channel_description)
                setShowBadge(false)
            }
            val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            nm.createNotificationChannel(channel)
        }
    }

    private fun buildNotification(): Notification {
        // Intent для открытия MainActivity при нажатии на уведомление
        val openIntent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val openPending = PendingIntent.getActivity(
            this, 0, openIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        // Intent для кнопки "Остановить"
        val stopIntent = Intent(this, TeleGhostService::class.java).apply {
            action = "STOP_SERVICE"
        }
        val stopPending = PendingIntent.getService(
            this, 1, stopIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.notification_title))
            .setContentText(getString(R.string.notification_text))
            .setSmallIcon(R.mipmap.ic_launcher_foreground) // TODO: заменить на кастомную иконку
            .setContentIntent(openPending)
            .setOngoing(true)          // Non-dismissible
            .setAutoCancel(false)      // Doesn't close on click
            .setShowWhen(false)
            .setPriority(NotificationCompat.PRIORITY_DEFAULT)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .addAction(
                android.R.drawable.ic_menu_close_clear_cancel,
                getString(R.string.notification_stop),
                stopPending
            )
            .build()
    }

    // ─── WakeLock ─────────────────────────────────────────────────────────

    private fun acquireWakeLock() {
        val pm = getSystemService(Context.POWER_SERVICE) as PowerManager
        wakeLock = pm.newWakeLock(
            PowerManager.PARTIAL_WAKE_LOCK,
            WAKELOCK_TAG
        ).apply {
            acquire(10 * 60 * 1000L) // 10 минут, будет обновляться
        }
        Log.d(TAG, "WakeLock acquired")
    }

    private fun releaseWakeLock() {
        wakeLock?.let {
            if (it.isHeld) {
                it.release()
                Log.d(TAG, "WakeLock released")
            }
        }
        wakeLock = null
    }
}
