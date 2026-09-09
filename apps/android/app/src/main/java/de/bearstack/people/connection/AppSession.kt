package de.bearstack.people.connection

import android.content.Context
import coil.ImageLoader
import coil.memory.MemoryCache
import coil.request.CachePolicy
import de.bearstack.people.data.remote.*
import de.bearstack.people.media.OriginalMemoryCache
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
        val images: ImageLoader, val photos: PhotosController?, internal val client: OkHttpClient) {
        suspend fun close() = withContext(NonCancellable + Dispatchers.IO) {
            try { images.memoryCache?.clear(); images.shutdown() }
            finally { Connections.close(client) }
        }
    }

    suspend fun restore(): Boolean {
        val profile = store.read() ?: return false
        open(profile, false)
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
        var published = false
        try {
            val people = LabelingApi(client, profile.url)
            val gallery = PhotosApi(client, profile.url)
            val gallerySession = try { gallery.session() } catch (e: ApiFailure) {
                if (e.status != 404) throw e
                null // Older servers can still provide person management.
            }
            val peopleSession = if (gallerySession?.canManagePeople != false) people.session() else null
            images = ImageLoader.Builder(context).okHttpClient(client).diskCachePolicy(CachePolicy.DISABLED)
                .memoryCache { OriginalMemoryCache(MemoryCache.Builder(context).maxSizeBytes(16 * 1024 * 1024)
                    .weakReferencesEnabled(false).build()) }.build()
            if (save) store.write(profile)
            currentCoroutineContext().ensureActive()
            // Do not replace a working account until negotiation and persistence
            // succeed. Detach feature jobs before disposing the old connection.
            detach()?.close()
            currentCoroutineContext().ensureActive()
            photos = gallerySession?.let { PhotosController(scope, gallery, it, context) }
            active = Resources(people, peopleSession, images, photos, client)
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
        detach()?.close()
        if (forget) store.clear()
    }
}
