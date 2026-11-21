package id.co.erayadigital.listener

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import androidx.work.workDataOf
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.TimeUnit

/**
 * Mengirim satu event ke webhook. Retry dengan backoff untuk gangguan jaringan/5xx/408/429; 4xx lain dianggap
 * permanen (URL/secret salah) dan tidak diulang. Satu event = satu pekerjaan unik per delivery id.
 */
class SendWorker(ctx: Context, params: WorkerParameters) : CoroutineWorker(ctx, params) {
    override suspend fun doWork(): Result {
        val cfg = Config(applicationContext)
        val body = inputData.getString("body") ?: return Result.failure()
        val id = inputData.getString("id") ?: return Result.failure()
        if (!cfg.urlValid() || cfg.secret.isEmpty()) {
            cfg.lastResult = "Gagal: URL webhook atau secret belum diisi"
            return Result.failure()
        }
        val ts = System.currentTimeMillis() / 1000
        val conn = URL(cfg.webhookUrl).openConnection() as HttpURLConnection
        return try {
            conn.requestMethod = "POST"
            conn.connectTimeout = 10_000
            conn.readTimeout = 15_000
            conn.doOutput = true
            conn.setRequestProperty("Content-Type", "application/json")
            conn.setRequestProperty("X-Listener-Delivery-Id", id)
            conn.setRequestProperty("X-Listener-Timestamp", ts.toString())
            conn.setRequestProperty("X-Listener-Signature", "sha256=" + Signing.signature(cfg.secret, ts, body))
            conn.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }
            val code = conn.responseCode
            when {
                code in 200..299 -> { cfg.lastResult = "Terkirim ($code) ${stamp()}"; Result.success() }
                code == 408 || code == 429 || code >= 500 -> { cfg.lastResult = "Server $code, akan diulang ${stamp()}"; Result.retry() }
                else -> { cfg.lastResult = "Ditolak $code: ${reject(code)} ${stamp()}"; Result.failure() }
            }
        } catch (e: java.io.IOException) {
            cfg.lastResult = "Jaringan gagal, akan diulang ${stamp()}"
            Result.retry()
        } finally {
            conn.disconnect()
        }
    }

    private fun reject(code: Int) = when (code) {
        400, 401, 403 -> "secret/tanda tangan tidak cocok atau jam HP salah; buat tautan setup baru di EraPay"
        404 -> "URL webhook salah"
        else -> "cek URL dan secret"
    }

    private fun stamp() = java.text.SimpleDateFormat("HH:mm:ss", java.util.Locale.US).format(java.util.Date())

    companion object {
        fun enqueue(ctx: Context, id: String, body: String) {
            val req = OneTimeWorkRequestBuilder<SendWorker>()
                .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 15, TimeUnit.SECONDS)
                .setInputData(workDataOf("id" to id, "body" to body))
                .build()
            // KEEP: notifikasi yang sama tidak mengantre dua kali.
            WorkManager.getInstance(ctx).enqueueUniqueWork("send-$id", ExistingWorkPolicy.KEEP, req)
        }
    }
}
