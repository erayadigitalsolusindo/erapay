package id.co.erayadigital.listener

import android.app.Notification
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification

/** Meneruskan notifikasi dari aplikasi yang namanya ada di daftar paket. Isi notifikasi tidak dicatat ke log. */
class ListenerService : NotificationListenerService() {
    override fun onNotificationPosted(sbn: StatusBarNotification) {
        val cfg = Config(this)
        if (!cfg.ready() || sbn.packageName !in cfg.packages) return
        val n = sbn.notification
        if (n.flags and Notification.FLAG_GROUP_SUMMARY != 0) return // ringkasan grup hanya duplikat isi
        val ex = n.extras
        val title = ex.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty()
        val text = (ex.getCharSequence(Notification.EXTRA_BIG_TEXT) ?: ex.getCharSequence(Notification.EXTRA_TEXT))?.toString().orEmpty()
        if (title.isEmpty() && text.isEmpty()) return
        val (id, body) = Events.build(cfg, "notification.posted", sbn.packageName, title, text, sbn.postTime)
        SendWorker.enqueue(this, id, body)
    }
}
