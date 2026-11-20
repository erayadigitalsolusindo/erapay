package id.co.erayadigital.listener

import java.security.MessageDigest
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/** Tanda tangan webhook: HMAC-SHA256 hex atas "<timestamp>." + body mentah, dengan secret bersama. */
object Signing {
    fun signature(secret: String, timestamp: Long, body: String): String =
        hex(Mac.getInstance("HmacSHA256").run {
            init(SecretKeySpec(secret.toByteArray(Charsets.UTF_8), "HmacSHA256"))
            doFinal("$timestamp.$body".toByteArray(Charsets.UTF_8))
        })

    /** Id kiriman yang stabil: notifikasi yang sama (dikirim ulang sistem) menghasilkan id yang sama. */
    fun deliveryId(pkg: String, postTime: Long, title: String, text: String): String =
        hex(MessageDigest.getInstance("SHA-256").digest("$pkg|$postTime|$title|$text".toByteArray(Charsets.UTF_8))).take(32)

    private fun hex(b: ByteArray) = b.joinToString("") { "%02x".format(it) }
}
