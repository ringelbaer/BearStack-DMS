package de.bearstack.people.connection

import de.bearstack.people.media.networkClient
import android.content.Context
import coil3.ImageLoader
import coil3.memory.MemoryCache
import coil3.request.CachePolicy
import de.bearstack.people.data.remote.*
import de.bearstack.people.media.OriginalMemoryCache
import de.bearstack.people.media.ThumbnailCache
import de.bearstack.people.photos.PhotosController
import kotlinx.coroutines.*
import okhttp3.OkHttpClient

/** Application connection lifetime, independent of the people queue and its database.
 * All state changes run on Main, as do the feature controllers using this session.
 */
class AppSession(private val context: Context, private val scope: CoroutineScope,
    private val beforeDisconnect: () -> Unit = {}) {
    private val store = ProfileStore(context)
    private var pendingProfile: Profile? = null
    var certificate: CertificateOffer? = null; private set
    var active: Resources? = null; private set

    class Resources internal constructor(val people: LabelingApi, val peopleSession: Session?,
        val images: ImageLoader, val photos: PhotosController?, internal val client: OkHttpClient,
        internal val thumbnails: ThumbnailCache? = null) {
        suspend fun close(clearThumbnails: Boolean = false) = withContext(NonCancellable + Dispatchers.IO) {
            try { thumbnails?.close(clearThumbnails); images.memoryCache?.clear(); images.shutdown() }
            finally { Connections.close(client) }
        }
    }

    suspend fun restore(onProfile: () -> Unit = {}): Boolean {
        val profile = store.read() ?: return false
        onProfile()
        // Bound the complete negotiation, including both session endpoints. Slow
        // or silent servers must not hold the local gallery behind sign-in.
        val restored = withTimeoutOrNull(8_000) { open(profile, false); true } ?: false
        if (!restored) throw ConnectionAttemptException(ConnectionStage.SIGN_IN,
            java.net.SocketTimeoutException("Session restoration timed out"))
        return true
    }

    suspend fun connect(url: String, username: String, password: String): Boolean {
        cancelCertificate()
        val profile = Profile(Connections.address(url).toString(), username.trim(), password)
        val offer = Connections.inspect(profile.url)
        if (offer != null) {
            pendingProfile = profile
            certificate = offer
            return false
        }
        open(profile, true)
        return true
    }

    suspend fun confirmCertificate(): Boolean {
        val profile = pendingProfile ?: return false
        val offer = certificate ?: return false
        open(profile.copy(certificate = offer.encoded), true)
        return true
    }

    fun cancelCertificate() { pendingProfile = null; certificate = null }

    private suspend fun open(profile: Profile, save: Boolean) {
        val client = Connections.client(profile)
        var images: ImageLoader? = null
        var photos: PhotosController? = null
        var thumbnails: ThumbnailCache? = null
        var published = false
        try {
            val people = LabelingApi(client, profile.url)
            val gallery = PhotosApi(client, profile.url)
            val gallerySession = try { gallery.session() } catch (e: ApiFailure) {
                if (e.status != 404) throw e
                null // Older servers can still provide person management.
            }
            val peopleSession = if (gallerySession?.canManagePeople != false) people.session() else null
            if (save) store.write(profile)
            currentCoroutineContext().ensureActive()
            // Do not replace a working account until negotiation and persistence
            // succeed. Detach feature jobs before disposing the old connection.
            detach()?.close()
            currentCoroutineContext().ensureActive()
            thumbnails = gallerySession?.let {
                try { ThumbnailCache.open(context, profile.url, client, gallery, it, scope) }
                catch (_: java.io.IOException) { null } // Keep the gallery usable; settings report unavailable storage.
            }
            images = ImageLoader.Builder(context).networkClient(client).diskCachePolicy(CachePolicy.DISABLED)
                .components { thumbnails?.let { add(ThumbnailCache.Factory(it)); add(ThumbnailCache.Keys()); add(ThumbnailCache.Integrity(it)) } }
                .memoryCache { OriginalMemoryCache(MemoryCache.Builder().maxSizeBytes(16 * 1024 * 1024)
                    .weakReferencesEnabled(false).build()) }.build()
            photos = gallerySession?.let { PhotosController(scope, gallery, it, context) }
            photos?.thumbnailCache = thumbnails
            active = Resources(people, peopleSession, images, photos, client, thumbnails)
            thumbnails?.refresh()
            published = true
            cancelCertificate()
        } catch (e: java.io.IOException) {
            if (e is ApiFailure) throw e
            throw ConnectionAttemptException(ConnectionStage.SIGN_IN, e)
        } finally {
            if (!published) {
                photos?.close()
                // Cancellation and persistence failures must also dispose the
                // unpublished client, without masking the original login error.
                withContext(NonCancellable) {
                    runCatching { thumbnails?.close() }
                    runCatching { images?.memoryCache?.clear(); images?.shutdown() }
                    runCatching { Connections.close(client) }
                }
            }
        }
    }

    fun detach(): Resources? {
        beforeDisconnect()
        val old = active
        active = null
        old?.photos?.close()
        cancelCertificate()
        return old
    }

    suspend fun disconnect(forget: Boolean = false) {
        detach()?.close(clearThumbnails = forget)
        if (forget) {
            store.clear()
            withContext(Dispatchers.IO) { java.io.File(context.noBackupFilesDir, "thumbnails-v1").deleteRecursively() }
        }
    }
}
