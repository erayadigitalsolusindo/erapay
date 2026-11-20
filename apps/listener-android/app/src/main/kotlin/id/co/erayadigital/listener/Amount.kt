package id.co.erayadigital.listener

/** Mengambil nominal rupiah pertama dari teks notifikasi ("Rp 1.234.567", "Rp1.234.567,00", "IDR 50.000"). null bila tak ada. */
object Amount {
    private val re = Regex("""(?i)(?:rp\.?|idr)\s*([0-9]{1,3}(?:\.[0-9]{3})+|[0-9]+)(?:,[0-9]{1,2})?""")

    fun parse(text: String): Long? {
        val digits = re.find(text)?.groupValues?.get(1)?.replace(".", "") ?: return null
        return digits.toLongOrNull()
    }
}
