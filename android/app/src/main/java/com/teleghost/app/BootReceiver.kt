package com.teleghost.app

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import android.util.Log

/**
 * BootReceiver — автозапуск TeleGhostService после перезагрузки устройства.
 * Опциональный компонент: пользователь может отключить через настройки.
 */
class BootReceiver : BroadcastReceiver() {

    companion object {
        private const val TAG = "TeleGhostBoot"
    }

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED) return

        Log.i(TAG, "Boot completed, starting TeleGhost service...")

        val serviceIntent = Intent(context, TeleGhostService::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            context.startForegroundService(serviceIntent)
        } else {
            context.startService(serviceIntent)
        }
    }
}
