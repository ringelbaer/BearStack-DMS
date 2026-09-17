package de.bearstack.people.media

import android.content.Context
import android.graphics.BitmapFactory
import coil.ImageLoader
import coil.decode.DataSource
import coil.decode.ImageSource
import coil.fetch.Fetcher
import coil.fetch.SourceResult
import coil.key.Keyer
import coil.intercept.Interceptor
import coil.request.ErrorResult
import coil.request.Options
import de.bearstack.people.data.remote.*
import java.io.File
import java.io.IOException
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.sync.withPermit
import okhttp3.OkHttpClient
import okhttp3.Request
import okio.Buffer

internal class ThumbnailPreferences(context: Context) {
    private val store = context.applicationContext.getSharedPreferences("thumbnail_cache", Context.MODE_PRIVATE)
    var sizeMiB: Int
        get() = store.getInt("size_mib", 256).coerceIn(64, 2048)
        set(value) { store.edit().putInt("size_mib", value.coerceIn(64, 2048)).apply() }
}

internal data class CachedThumbnail(val url: String, val revision: String) {
    val key = thumbnailDigest("$url\n$revision")
}

internal fun cachedThumbnail(service: PhotosService, session: PhotoSession, photo: Photo, size: Int) =
    CachedThumbnail(service.thumbnail(photo, size), session.scope + ":" + photo.version)

internal data class ThumbnailCacheState(val usage: ThumbnailCacheUsage, val syncing: Boolean = false,
    val failed: Boolean = false)

/** Session-owned cache and cancellable warmer. UI loads and warming share the same
 * per-key locks and three download slots; originals never enter this cache.
 */
internal class ThumbnailCache(private val disk: ThumbnailDiskCache, private val client: OkHttpClient,
    private val service: PhotosService, private val session: PhotoSession, parent: CoroutineScope) {
    private val scope = CoroutineScope(parent.coroutineContext + SupervisorJob(parent.coroutineContext[Job]) + Dispatchers.IO)
    private val locks = Array(32) { Mutex() }
    private val downloads = Semaphore(3)
    private val mutable = MutableStateFlow(ThumbnailCacheState(disk.usage()))
    val state = mutable.asStateFlow()
    private var refreshJob: Job? = null
    private var lastRefresh = 0L

    suspend fun load(data: CachedThumbnail): Pair<ByteArray, DataSource> {
        val request = scope.async { loadOnIo(data) }
        try { return request.await() }
        finally { if (!currentCoroutineContext().isActive) request.cancel() }
    }

    private suspend fun loadOnIo(data: CachedThumbnail): Pair<ByteArray, DataSource> =
        locks[(data.key.hashCode() and Int.MAX_VALUE) % locks.size].withLock {
            disk.read(data.key)?.let { return@withLock it to DataSource.DISK }
            val bytes = downloads.withPermit {
                client.readResponse(Request.Builder().url(data.url).build()) { response, _ ->
                    if (response.code != 200) throw IOException("Thumbnail HTTP ${response.code}")
                    val body = response.body ?: throw IOException("Empty thumbnail")
                    if (body.contentType()?.type != "image") throw IOException("Invalid thumbnail type")
                    val source = body.source()
                    if (source.request(THUMBNAIL_MAX_BYTES + 1)) throw IOException("Thumbnail too large")
                    val value = source.readByteArray()
                    val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
                    BitmapFactory.decodeByteArray(value, 0, value.size, bounds)
                    if (bounds.outWidth !in 1..640 || bounds.outHeight !in 1..640) throw IOException("Invalid thumbnail dimensions")
                    value
                }
            }
            currentCoroutineContext().ensureActive()
            try { disk.write(data.key, bytes); publish() }
            catch (_: IOException) { publish(failed = true) }
            bytes to DataSource.NETWORK
        }

    @Synchronized fun refresh(force: Boolean = false) {
        if (refreshJob?.isActive == true) return
        val now = System.nanoTime()
        if (!force && lastRefresh != 0L && now - lastRefresh < 60_000_000_000L) return
        lastRefresh = now
        refreshJob = scope.launch {
            publish(syncing = true, failed = false)
            try {
                val required = requiredThumbnails(service, session)
                disk.protect(required.keys)
                publish()
                // Serial warming leaves two slots for foreground image requests.
                for (thumbnail in required.values) {
                    ensureActive()
                    try { load(thumbnail) }
                    catch (e: CancellationException) { throw e }
                    catch (_: Exception) { publish(failed = true) }
                }
            } catch (e: CancellationException) { throw e }
            catch (_: Exception) { publish(failed = true) }
            finally { publish(syncing = false) }
        }
    }

    fun resize(sizeMiB: Int) { scope.launch {
        try { disk.resize(sizeMiB.coerceIn(64, 2048) * MIB); publish() }
        catch (_: IOException) { publish(failed = true) }
    } }
    suspend fun close(clear: Boolean = false) {
        scope.cancel()
        withContext(NonCancellable + Dispatchers.IO) { scope.coroutineContext[Job]?.join(); disk.close(clear) }
    }
    @Synchronized private fun publish(syncing: Boolean = mutable.value.syncing, failed: Boolean = mutable.value.failed) {
        mutable.value = ThumbnailCacheState(disk.usage(), syncing, failed)
    }

    class Factory(private val cache: ThumbnailCache) : Fetcher.Factory<CachedThumbnail> {
        override fun create(data: CachedThumbnail, options: Options, imageLoader: ImageLoader) = Fetcher {
            val (bytes, source) = cache.load(data)
            SourceResult(ImageSource(Buffer().write(bytes), options.context), null, source)
        }
    }
    class Keys : Keyer<CachedThumbnail> {
        override fun key(data: CachedThumbnail, options: Options) = data.key
    }
    // Header validation bounds allocations, but cannot detect all corrupt pixel
    // data. A decoder failure must not leave a permanently broken disk entry.
    class Integrity(private val cache: ThumbnailCache) : Interceptor {
        override suspend fun intercept(chain: Interceptor.Chain): coil.request.ImageResult {
            val result = chain.proceed(chain.request)
            val data = chain.request.data as? CachedThumbnail
            if (data != null && result is ErrorResult) withContext(Dispatchers.IO) {
                cache.disk.remove(data.key)
                cache.publish(failed = true)
            }
            return result
        }
    }

    companion object {
        suspend fun open(context: Context, address: String, client: OkHttpClient, service: PhotosService,
            session: PhotoSession, parent: CoroutineScope): ThumbnailCache = withContext(Dispatchers.IO) {
            val root = File(context.noBackupFilesDir, "thumbnails-v1")
            val directory = File(root, thumbnailDigest(address + "\n" + session.scope))
            root.listFiles()?.filter { it != directory }?.forEach { it.deleteRecursively() }
            ThumbnailCache(ThumbnailDiskCache(directory, ThumbnailPreferences(context).sizeMiB * MIB), client, service, session, parent)
        }
    }
}

/** Metadata pages stay bounded, even with thousands of root folders. Keep only
 * the distinct required URLs, never complete folder/photo catalogues.
 * Commit a new pin set only after every metadata page succeeds.
 */
internal suspend fun requiredThumbnails(service: PhotosService, session: PhotoSession): Map<String, CachedThumbnail> {
    val required = LinkedHashMap<String, CachedThumbnail>()
    fun add(photo: Photo, size: Int) {
        val item = cachedThumbnail(service, session, photo, size)
        required[item.key] = item
    }
    val stream = service.browse(PhotoQuery(recursive = true), 1, "media")
    require(stream.page == 1 && stream.media.size <= 96)
    stream.media.take(50).forEach { add(it, session.thumbnailSize) }
    var number = 1
    do {
        currentCoroutineContext().ensureActive()
        val page = service.browse(PhotoQuery(), number, "folders")
        require(page.page == number && page.folders.size <= 24 &&
            page.folders.all { it.previews.size <= if (it.virtual) 8 else 4 })
        page.folders.forEach { folder -> folder.previews.forEach { add(it, session.folderThumbnailSize) } }
        if (!page.folderHasNext) break
        require(page.folders.isNotEmpty() && number < 1_000_000)
        number++
    } while (true)
    return required
}
