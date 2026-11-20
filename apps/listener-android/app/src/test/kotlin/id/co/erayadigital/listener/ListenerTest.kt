package id.co.erayadigital.listener

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ListenerTest {
    @Test fun amountFormats() {
        assertEquals(1_234_567L, Amount.parse("Anda menerima Rp 1.234.567 dari BUDI"))
        assertEquals(1_234_567L, Amount.parse("Rp1.234.567,00 masuk"))
        assertEquals(50_000L, Amount.parse("IDR 50.000"))
        assertEquals(10_000L, Amount.parse("saldo bertambah rp. 10000"))
        assertEquals(999L, Amount.parse("Rp 999"))
        assertNull(Amount.parse("Tidak ada nominal di sini"))
    }

    @Test fun signatureMatchesReference() {
        // Vektor dari `openssl dgst -sha256 -hmac key` atas "1700000000.{}".
        assertEquals("9d713ed406bb7076d4123f0dc2c39d2df5c654ed4b0cd56b52c8b4c940bd63ae", Signing.signature("key", 1700000000, "{}"))
        assertEquals(Signing.signature("k", 1, "a"), Signing.signature("k", 1, "a"))
        assertNotEquals(Signing.signature("k", 1, "a"), Signing.signature("k", 2, "a"))
        assertNotEquals(Signing.signature("k", 1, "a"), Signing.signature("k2", 1, "a"))
    }

    @Test fun deliveryIdIsStable() {
        assertEquals(Signing.deliveryId("p", 1, "t", "x"), Signing.deliveryId("p", 1, "t", "x"))
        assertNotEquals(Signing.deliveryId("p", 1, "t", "x"), Signing.deliveryId("p", 2, "t", "x"))
    }
}
