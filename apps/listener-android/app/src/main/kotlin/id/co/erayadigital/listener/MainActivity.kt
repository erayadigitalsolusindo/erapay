package id.co.erayadigital.listener

import android.app.Activity
import android.content.ComponentName
import android.content.Intent
import android.os.Bundle
import android.provider.Settings
import android.service.notification.NotificationListenerService
import android.text.InputType
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast

/** Satu layar: URL webhook, secret, daftar nama paket, izin akses notifikasi, dan tombol tes. */
class MainActivity : Activity() {
    private lateinit var cfg: Config
    private lateinit var url: EditText
    private lateinit var secret: EditText
    private lateinit var packages: EditText
    private lateinit var device: EditText
    private lateinit var status: TextView

    override fun onCreate(b: Bundle?) {
        super.onCreate(b)
        cfg = Config(this)
        val pad = (16 * resources.displayMetrics.density).toInt()
        val col = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL; setPadding(pad, pad, pad, pad) }
        fun label(t: String) = TextView(this).apply { text = t; setPadding(0, pad / 2, 0, 0) }.also { col.addView(it) }
        fun field(v: String, hint: String, lines: Int = 1, type: Int = InputType.TYPE_CLASS_TEXT) = EditText(this).apply {
            setText(v); this.hint = hint; inputType = type or (if (lines > 1) InputType.TYPE_TEXT_FLAG_MULTI_LINE else 0)
            minLines = lines; isSingleLine = lines == 1
        }.also { col.addView(it, ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT) }
        fun button(t: String, on: () -> Unit) = Button(this).apply { text = t; setOnClickListener { on() } }.also { col.addView(it) }

        col.addView(TextView(this).apply { text = "EraPay Listener"; textSize = 22f })
        status = TextView(this).also { col.addView(it) }
        label("URL webhook (https)")
        url = field(cfg.webhookUrl, "https://arus.example.com/webhooks/payment-listener", type = InputType.TYPE_TEXT_VARIATION_URI)
        label("Secret bersama (min. 16 karakter)")
        secret = field(cfg.secret, "isi sama dengan di sisi penerima", type = InputType.TYPE_TEXT_VARIATION_PASSWORD)
        label("Nama paket aplikasi (satu per baris)")
        packages = field(cfg.packages.joinToString("\n"), "com.gojek.app\nid.dana\ncom.bca", lines = 4)
        label("Nama perangkat (opsional)")
        device = field(cfg.deviceName, "mis. HP kasir 1")
        button("Simpan") { save() }
        button("1. Beri izin akses notifikasi") { startActivity(Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS)) }
        button("2. Matikan penghemat baterai untuk aplikasi ini") { startActivity(Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)) }
        button("Kirim tes ke webhook") { if (save()) { Events.sendTest(this, cfg); toast("Tes diantrekan") } }
        setContentView(ScrollView(this).apply { addView(col) })
    }

    override fun onResume() {
        super.onResume()
        refresh()
    }

    private fun save(): Boolean {
        val u = url.text.toString().trim()
        val s = secret.text.toString().trim()
        val p = packages.text.toString().lines().map { it.trim() }.filter { it.isNotEmpty() }
        val bad = p.firstOrNull { !it.matches(Regex("[A-Za-z][A-Za-z0-9_]*(\\.[A-Za-z][A-Za-z0-9_]*)+")) }
        when {
            !cfg.urlValid(u) -> toast("URL harus diawali https://")
            s.length < 16 -> toast("Secret minimal 16 karakter")
            p.isEmpty() -> toast("Isi minimal satu nama paket")
            bad != null -> toast("Nama paket tidak valid: $bad")
            else -> {
                cfg.webhookUrl = u; cfg.secret = s; cfg.packages = p.toSet(); cfg.deviceName = device.text.toString()
                // Minta sistem mengikat ulang layanan agar perubahan langsung berlaku.
                NotificationListenerService.requestRebind(ComponentName(this, ListenerService::class.java))
                refresh()
                toast("Tersimpan")
                return true
            }
        }
        return false
    }

    private fun refresh() {
        val granted = Settings.Secure.getString(contentResolver, "enabled_notification_listeners")?.contains(packageName) == true
        status.text = buildString {
            append(if (granted) "Izin notifikasi: AKTIF" else "Izin notifikasi: BELUM (tekan tombol 1)")
            append("\nKonfigurasi: ").append(if (cfg.ready()) "lengkap" else "belum lengkap")
            if (cfg.lastResult.isNotEmpty()) append("\nKiriman terakhir: ").append(cfg.lastResult)
        }
    }

    private fun toast(t: String) = Toast.makeText(this, t, Toast.LENGTH_SHORT).show()
}
