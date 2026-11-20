package id.co.erayadigital.listener

import android.content.Context
import org.json.JSONObject
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

object Events {
    private fun iso(t: Long) = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US).apply { timeZone = TimeZone.getTimeZone("UTC") }.format(Date(t))

    /** Body JSON webhook. `amount` hanya tebakan dari teks (null bila tak ada); `text` mentah tetap sumber kebenaran. */
    fun build(cfg: Config, event: String, pkg: String, title: String, text: String, postTime: Long): Pair<String, String> {
        val id = Signing.deliveryId(pkg, postTime, title, text)
        val json = JSONObject()
            .put("event", event)
            .put("delivery_id", id)
            .put("device_id", cfg.deviceId)
            .put("device_name", cfg.deviceName)
            .put("package", pkg)
            .put("title", title)
            .put("text", text)
            .put("amount", Amount.parse("$title $text") ?: JSONObject.NULL)
            .put("posted_at", iso(postTime))
        return id to json.toString()
    }

    fun sendTest(ctx: Context, cfg: Config) {
        val (id, body) = build(cfg, "listener.test", ctx.packageName, "Tes EraPay Listener", "Rp 10.000 masuk (tes)", System.currentTimeMillis())
        SendWorker.enqueue(ctx, id, body)
    }
}
