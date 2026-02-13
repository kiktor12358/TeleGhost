package com.teleghost.app

import android.annotation.SuppressLint
import android.app.Activity
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.view.View
import android.view.WindowManager
import android.webkit.*
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import java.io.File
import java.io.FileOutputStream
import java.util.UUID

/**
 * MainActivity — главная Activity приложения.
 *
 * Архитектура:
 * 1. Стартует TeleGhostService (Foreground Service с I2P)
 * 2. Ждёт готовности Go HTTP сервера (health check)
 * 3. Загружает WebView на http://127.0.0.1:8080
 *
 * WebView отображает тот же Svelte-фронтенд, что и Wails на десктопе.
 * Api Bridge (api_bridge.js) автоматически переключается на HTTP.
 *
 * Update: Implements mobile.PlatformBridge for native file selection.
 */
class MainActivity : AppCompatActivity(), mobile.PlatformBridge {

    private lateinit var webView: WebView

    companion object {
        private const val TAG = "TeleGhost"
        private const val SERVER_URL = "http://127.0.0.1:8080"
        private const val HEALTH_URL = "$SERVER_URL/health"
        private const val MAX_RETRIES = 30
        private const val RETRY_DELAY_MS = 1000L
        private const val NOTIFICATION_CHANNEL_ID = "teleghost_messages"
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val name = "Messages"
            val descriptionText = "New messages from contacts"
            val importance = android.app.NotificationManager.IMPORTANCE_DEFAULT
            val channel = android.app.NotificationChannel(NOTIFICATION_CHANNEL_ID, name, importance).apply {
                description = descriptionText
            }
            val notificationManager: android.app.NotificationManager =
                getSystemService(android.content.Context.NOTIFICATION_SERVICE) as android.app.NotificationManager
            notificationManager.createNotificationChannel(channel)
        }
    }

    // File Picker Launcher
    private val filePickerLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        if (result.resultCode == Activity.RESULT_OK) {
            val uri = result.data?.data
            if (uri != null) {
                // Copy in background to avoid freezing UI
                android.util.Log.i(TAG, "File selected: $uri")
                Thread {
                    try {
                        val path = copyFileToInternalStorage(uri)
                        android.util.Log.i(TAG, "File copied to: $path")
                        mobile.Mobile.onFileSelected(path)
                    } catch (e: Exception) {
                        android.util.Log.e(TAG, "Failed to copy file", e)
                        mobile.Mobile.onFileSelected("") // Signal error/cancel
                    }
                }.start()
            } else {
                mobile.Mobile.onFileSelected("")
            }
        } else {
            android.util.Log.i(TAG, "File selection canceled")
            mobile.Mobile.onFileSelected("")
        }
    }

    // Notification Permission Launcher
    private val requestPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { isGranted: Boolean ->
        if (isGranted) {
            android.util.Log.i(TAG, "Notification permission granted")
        } else {
            android.util.Log.w(TAG, "Notification permission denied")
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Fullscreen immersive mode
        window.decorView.systemUiVisibility = (
            View.SYSTEM_UI_FLAG_LAYOUT_STABLE
            or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
        )
        window.statusBarColor = android.graphics.Color.TRANSPARENT
        window.navigationBarColor = android.graphics.Color.parseColor("#0c0c14")

        // Не давать Android убить Activity при нехватки памяти
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE)

        // Создаём WebView программно (без XML layout)
        webView = WebView(this).apply {
            layoutParams = android.view.ViewGroup.LayoutParams(
                android.view.ViewGroup.LayoutParams.MATCH_PARENT,
                android.view.ViewGroup.LayoutParams.MATCH_PARENT
            )
            setBackgroundColor(android.graphics.Color.parseColor("#0c0c14"))
        }
        setContentView(webView)

        setupWebView()
        startGoService()

        // Create notification channel
        createNotificationChannel()

        // Request notification permission for Android 13+
        if (Build.VERSION.SDK_INT >= 33) {
            if (checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) !=
                android.content.pm.PackageManager.PERMISSION_GRANTED) {
                requestPermissionLauncher.launch(android.Manifest.permission.POST_NOTIFICATIONS)
            }
        }

        // Register this activity as the Native Bridge for Go
        mobile.Mobile.setPlatformBridge(this)
    }

    // --- PlatformBridge Implementation ---
    override fun openFile(path: String) {
        runOnUiThread {
            try {
                val file = File(path)
                if (!file.exists()) {
                    android.widget.Toast.makeText(this, "File not found: $path", android.widget.Toast.LENGTH_SHORT).show()
                    return@runOnUiThread
                }

                val uri = androidx.core.content.FileProvider.getUriForFile(
                    this,
                    "com.teleghost.app.fileprovider",
                    file
                )

                val intent = Intent(Intent.ACTION_VIEW).apply {
                    setDataAndType(uri, getMimeType(path))
                    addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                }

                startActivity(Intent.createChooser(intent, "Open File"))
            } catch (e: Exception) {
                android.util.Log.e(TAG, "Failed to open file", e)
                android.widget.Toast.makeText(this, "Open failed: ${e.message}", android.widget.Toast.LENGTH_SHORT).show()
            }
        }
    }

    private fun getMimeType(url: String): String {
        var type: String? = null
        val extension = MimeTypeMap.getFileExtensionFromUrl(url)
        if (extension != null) {
            type = MimeTypeMap.getSingleton().getMimeTypeFromExtension(extension)
        }
        return type ?: "*/*"
    }

    override fun pickFile() {
        runOnUiThread {
            try {
                val intent = Intent(Intent.ACTION_GET_CONTENT).apply {
                    type = "*/*"
                    addCategory(Intent.CATEGORY_OPENABLE)
                    putExtra(Intent.EXTRA_MIME_TYPES, arrayOf("image/*", "video/*", "application/pdf", "text/*", "application/zip"))
                }
                filePickerLauncher.launch(intent)
            } catch (e: Exception) {
                android.util.Log.e(TAG, "Failed to launch picker", e)
                mobile.Mobile.onFileSelected("")
            }
        }
    }

    override fun shareFile(path: String) {
        runOnUiThread {
            try {
                val file = File(path)
                if (!file.exists()) {
                    android.widget.Toast.makeText(this, "File not found: $path", android.widget.Toast.LENGTH_SHORT).show()
                    return@runOnUiThread
                }

                // Use FileProvider to share internal file
                val uri = androidx.core.content.FileProvider.getUriForFile(
                    this,
                    "com.teleghost.app.fileprovider",
                    file
                )

                val intent = Intent(Intent.ACTION_SEND).apply {
                    type = "application/zip"
                    putExtra(Intent.EXTRA_STREAM, uri)
                    addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                }

                startActivity(Intent.createChooser(intent, "Share Reseed File"))
            } catch (e: Exception) {
                android.util.Log.e(TAG, "Failed to share file", e)
                android.widget.Toast.makeText(this, "Share failed: ${e.message}", android.widget.Toast.LENGTH_SHORT).show()
            }
        }
    }

    override fun clipboardSet(text: String) {
        runOnUiThread {
            try {
                val clipboard = getSystemService(android.content.Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
                val clip = android.content.ClipData.newPlainText("TeleGhost", text)
                clipboard.setPrimaryClip(clip)
            } catch (e: Exception) {
                android.util.Log.e(TAG, "Clipboard set failed", e)
            }
        }
    }

    override fun showNotification(title: String, message: String) {
        runOnUiThread {
            try {
                // Check permission for Android 13+
                if (Build.VERSION.SDK_INT >= 33) {
                     if (checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != android.content.pm.PackageManager.PERMISSION_GRANTED) {
                         // Request permission? For now just log and skip or maybe request.
                         // Requesting permission inside notification call is bad UX.
                         // Ideally we should request it on startup.
                         // Let's assume user granted it or strict mode off for now.
                         // But we should try to show if possible.
                         // Actually, let's just try building it, if permission denied it will be ignored by system.
                     }
                }

                val builder = androidx.core.app.NotificationCompat.Builder(this, NOTIFICATION_CHANNEL_ID)
                    .setSmallIcon(android.R.drawable.ic_dialog_email) // Replace with app icon if available: R.mipmap.ic_launcher
                    .setContentTitle(title)
                    .setContentText(message)
                    .setPriority(androidx.core.app.NotificationCompat.PRIORITY_DEFAULT)
                    .setAutoCancel(true)
                    .setOngoing(true) // Make notification persistent

                // PendingIntent to open app when clicked
                val intent = Intent(this, MainActivity::class.java).apply {
                    flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK
                }
                val pendingIntent: android.app.PendingIntent = android.app.PendingIntent.getActivity(this, 0, intent, android.app.PendingIntent.FLAG_IMMUTABLE)
                builder.setContentIntent(pendingIntent)

                with(androidx.core.app.NotificationManagerCompat.from(this)) {
                    // notificationId is a unique int for each notification that you must define
                    // using current time as ID to show multiple notifications
                    notify(System.currentTimeMillis().toInt(), builder.build())
                }
            } catch (e: Exception) {
                android.util.Log.e(TAG, "Notification failed", e)
            }
        }
    }

    override fun saveFile(path: String, filename: String) {
        runOnUiThread {
            try {
                val file = File(path)
                if (!file.exists()) {
                     android.widget.Toast.makeText(this, "File to save not found", android.widget.Toast.LENGTH_SHORT).show()
                     return@runOnUiThread
                }

                // Use MediaStore to save to Downloads
                val contentValues = android.content.ContentValues().apply {
                    put(android.provider.MediaStore.MediaColumns.DISPLAY_NAME, filename)
                    // Try to detect mime type?
                    val mime = try {
                        val ext = MimeTypeMap.getFileExtensionFromUrl(filename)
                        MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext) ?: "application/octet-stream"
                    } catch(e: Exception) { "application/octet-stream" }
                    put(android.provider.MediaStore.MediaColumns.MIME_TYPE, mime)
                    
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                        put(android.provider.MediaStore.MediaColumns.RELATIVE_PATH, android.os.Environment.DIRECTORY_DOWNLOADS)
                        put(android.provider.MediaStore.MediaColumns.IS_PENDING, 1) // Mark as pending while writing
                    }
                }

                val resolver = applicationContext.contentResolver
                val uri = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                     resolver.insert(android.provider.MediaStore.Downloads.EXTERNAL_CONTENT_URI, contentValues)
                } else {
                     // Legacy storage? Not fully implementing legacy generic handling here for brevity, 
                     // but MediaStore.Downloads.EXTERNAL_CONTENT_URI might work on older versions too if targeting API 29? 
                     // Actually for < Q effectively requires WRITE_EXTERNAL_STORAGE.
                     // Assuming we are targeting >= 29 or ignoring legacy full support for now.
                     // But let's try external.
                     resolver.insert(android.provider.MediaStore.Files.getContentUri("external"), contentValues)
                }

                if (uri != null) {
                    resolver.openOutputStream(uri)?.use { output ->
                        java.io.FileInputStream(file).use { input ->
                            input.copyTo(output)
                        }
                    }

                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                        contentValues.clear()
                        contentValues.put(android.provider.MediaStore.MediaColumns.IS_PENDING, 0)
                        resolver.update(uri, contentValues, null, null)
                    }
                    android.widget.Toast.makeText(this, "Saved to Downloads", android.widget.Toast.LENGTH_SHORT).show()
                } else {
                    android.widget.Toast.makeText(this, "Failed to create MediaStore entry", android.widget.Toast.LENGTH_SHORT).show()
                }

            } catch (e: Exception) {
                android.util.Log.e(TAG, "Save file failed", e)
                android.widget.Toast.makeText(this, "Save failed: ${e.message}", android.widget.Toast.LENGTH_SHORT).show()
            }
        }
    }

    private fun copyFileToInternalStorage(uri: Uri): String {
        val contentResolver = applicationContext.contentResolver
        val mimeType = contentResolver.getType(uri) ?: "application/octet-stream"
        
        // Try to guess extension
        val ext = MimeTypeMap.getSingleton().getExtensionFromMimeType(mimeType) ?: "bin"
        val filename = "upload_${UUID.randomUUID()}.$ext"

        val tempDir = File(filesDir, "tmp_uploads")
        if (!tempDir.exists()) tempDir.mkdirs()
        
        // Clean old files (optional, maybe older than 1 day)
        // ...

        val destFile = File(tempDir, filename)
        
        contentResolver.openInputStream(uri)?.use { input ->
            FileOutputStream(destFile).use { output ->
                input.copyTo(output)
            }
        } ?: throw Exception("Cannot open input stream")

        return destFile.absolutePath
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun setupWebView() {
        webView.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true
            databaseEnabled = true
            allowFileAccess = true
            allowContentAccess = true
            mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
            cacheMode = WebSettings.LOAD_DEFAULT
            mediaPlaybackRequiresUserGesture = false
            useWideViewPort = true
            loadWithOverviewMode = true

            // Viewport settings
            setSupportZoom(false)
            builtInZoomControls = false
            displayZoomControls = false
        }

        webView.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(
                view: WebView?,
                request: WebResourceRequest?
            ): Boolean {
                // Все запросы к localhost обрабатываем внутри WebView
                val url = request?.url?.toString() ?: return false
                if (url.startsWith(SERVER_URL)) return false
                // Внешние ссылки — открываем в браузере
                startActivity(Intent(Intent.ACTION_VIEW, request?.url))
                return true
            }

            override fun onReceivedError(
                view: WebView?,
                request: WebResourceRequest?,
                error: WebResourceError?
            ) {
                if (request?.isForMainFrame == true) {
                    // Сервер ещё не стартовал — повторяем
                    android.util.Log.w(TAG, "WebView error: ${error?.description}, retrying...")
                    view?.postDelayed({ waitForServerAndLoad() }, RETRY_DELAY_MS)
                }
            }
        }

        webView.webChromeClient = object : WebChromeClient() {
            override fun onConsoleMessage(msg: ConsoleMessage?): Boolean {
                android.util.Log.d(TAG, "JS: ${msg?.message()} [${msg?.sourceId()}:${msg?.lineNumber()}]")
                return true
            }
        }
    }

    private fun startGoService() {
        val intent = Intent(this, TeleGhostService::class.java)

        // Запускаем как Foreground Service
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(intent)
        } else {
            startService(intent)
        }

        // Ждём пока сервер стартует, затем загружаем WebView
        waitForServerAndLoad()
    }

    /**
     * Polling цикл: проверяет /health эндпоинт Go сервера.
     * Как только сервер ответит — загружаем фронтенд.
     */
    private fun waitForServerAndLoad(retryCount: Int = 0) {
        if (retryCount >= MAX_RETRIES) {
            android.util.Log.e(TAG, "Server failed to start after $MAX_RETRIES retries")
            runOnUiThread {
                webView.loadData(
                    """
                    <html><body style="background:#0c0c14;color:#fff;font-family:sans-serif;
                    display:flex;align-items:center;justify-content:center;height:100vh;margin:0;">
                    <div style="text-align:center;">
                        <h2>⚠️ Не удалось запустить сервер</h2>
                        <p>Попробуйте перезапустить приложение</p>
                    </div></body></html>
                    """.trimIndent(),
                    "text/html", "UTF-8"
                )
            }
            return
        }

        Thread {
            try {
                val url = java.net.URL(HEALTH_URL)
                val connection = url.openConnection() as java.net.HttpURLConnection
                connection.connectTimeout = 1000
                connection.readTimeout = 1000
                connection.requestMethod = "GET"

                val responseCode = connection.responseCode
                connection.disconnect()

                if (responseCode == 200) {
                    android.util.Log.i(TAG, "Server is ready! Loading WebView...")
                    runOnUiThread {
                        webView.loadUrl(SERVER_URL)
                    }
                    return@Thread
                }
            } catch (e: Exception) {
                android.util.Log.d(TAG, "Health check failed (attempt ${retryCount + 1}): ${e.message}")
            }

            // Повтор через секунду
            runOnUiThread {
                webView.postDelayed({ waitForServerAndLoad(retryCount + 1) }, RETRY_DELAY_MS)
            }
        }.start()
    }

    override fun onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack()
        } else {
            // Сворачиваем приложение вместо закрытия (service продолжает работать)
            moveTaskToBack(true)
        }
    }

    override fun onDestroy() {
        webView.destroy()
        super.onDestroy()
    }

    override fun onResume() {
        super.onResume()
        webView.onResume()
        // Re-register just in case
        mobile.Mobile.setPlatformBridge(this)
    }

    override fun onPause() {
        webView.onPause()
        super.onPause()
    }
}
