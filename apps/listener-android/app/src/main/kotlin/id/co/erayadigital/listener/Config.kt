package id.co.erayadigital.listener

import android.content.Context
import java.util.UUID

/** Pengaturan perangkat. Disimpan lokal (allowBackup mati); secret tidak pernah dicatat ke log. */
class Config(context: Context) {
    private val p = context.applicationContext.getSharedPreferences("config", Context.MODE_PRIVATE)

    var webhookUrl: String
        get() = p.getString("url", "") ?: ""
        set(v) = p.edit().putString("url", v.trim()).apply()

    var secret: String
        get() = p.getString("secret", "") ?: ""
        set(v) = p.edit().putString("secret", v.trim()).apply()

    /** Nama paket aplikasi yang notifikasinya diteruskan (satu per baris di layar). */
    var packages: Set<String>
        get() = (p.getString("packages", "") ?: "").split('\n').map { it.trim() }.filter { it.isNotEmpty() }.toSet()
        set(v) = p.edit().putString("packages", v.joinToString("\n")).apply()

    var deviceName: String
        get() = p.getString("device_name", "") ?: ""
        set(v) = p.edit().putString("device_name", v.trim()).apply()

    val deviceId: String
        get() = p.getString("device_id", null) ?: UUID.randomUUID().toString().also { p.edit().putString("device_id", it).apply() }

    var lastResult: String
        get() = p.getString("last", "") ?: ""
        set(v) = p.edit().putString("last", v).apply()

    companion object {
        /** Isian awal nama paket: BCA, Mandiri, BRI, BNI, BSI, CIMB Niaga, GoPay, DANA, OVO, ShopeePay, LinkAja. */
        val DEFAULT_PACKAGES = listOf(
            "com.bca", "com.bca.mybca.omni.android", "id.bmri.livin", "id.co.bri.brimo", "src.com.bni", "id.bni.wondr",
            "com.bsm.activity2", "id.co.cimbniaga.mobile.android", "com.gojek.app", "id.dana", "ovo.id", "com.shopee.id", "com.telkom.mwallet"
        )
    }

    /** Hanya HTTPS (cleartext dimatikan di manifest); nilai lain ditolak di layar pengaturan. */
    fun urlValid(url: String = webhookUrl) = url.startsWith("https://") && url.length > 12
    fun ready() = urlValid() && secret.length >= 16 && packages.isNotEmpty()
}
