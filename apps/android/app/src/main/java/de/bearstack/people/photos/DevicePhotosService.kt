package de.bearstack.people.photos

import android.content.ContentResolver
import android.content.ContentUris
import android.database.Cursor
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.CancellationSignal
import android.provider.MediaStore
import de.bearstack.people.R
import de.bearstack.people.data.remote.*
import de.bearstack.people.text.UiText
import de.bearstack.people.text.UserIoFailure
import kotlinx.coroutines.*
import java.time.Instant
import java.time.ZoneId
import java.security.MessageDigest
import java.util.concurrent.atomic.AtomicLong
import kotlin.random.Random

/** Read-only, screen-scoped MediaStore catalog. No server client or filesystem traversal. */
internal class DevicePhotosService(private val resolver: ContentResolver, private val random: Random = Random.Default) : PhotosService {
    private val collection = MediaStore.Images.Media.EXTERNAL_CONTENT_URI
    private data class Catalog(val folders: List<PhotoFolder>, val buckets: Map<String, Pair<String, String?>>,
        val revision: ByteArray)
    @Volatile private var catalog: Catalog? = null
    private data class PlaybackOrder(val folder: String, val generation: Long, val ids: LongArray)
    private val playbackGeneration = AtomicLong()
    @Volatile private var playbackOrder: PlaybackOrder? = null

    override fun clearPlaybackOrder() { playbackGeneration.incrementAndGet(); playbackOrder=null }

    override suspend fun session() = SESSION
    override fun thumbnail(photo: Photo, size: Int) = photo.path
    override fun original(photo: Photo) = photo.path
    override suspend fun blog(path: String): PhotoBlog = throw UnsupportedOperationException()

    override suspend fun browse(query: PhotoQuery, page: Int, section: String): PhotoPage = read { signal ->
        require(page > 0)
        val current = catalog ?: scanFolders(signal).also { catalog = it }
        val folders = current.folders
        if(query.path.isEmpty()) {
            val start = ((page - 1).toLong() * 24).coerceAtMost(folders.size.toLong()).toInt()
            PhotoPage("", "", page, 0, false, folders.size, start + 24 < folders.size, false,
                emptyList(), folders.subList(start, (start + 24).coerceAtMost(folders.size)), emptyList())
        } else {
            val bucket = current.buckets[query.path] ?: throw UserIoFailure(UiText(R.string.error_missing))
            val count = folders.first { it.path == query.path }.count
            val selection = MediaStore.Images.Media.BUCKET_ID + " = ?" +
                if(bucket.second != null) " AND ${MediaStore.MediaColumns.VOLUME_NAME} = ?" else ""
            val args = listOfNotNull(bucket.first, bucket.second).toTypedArray()
            val offset = Math.multiplyExact(page - 1, 96)
            if(query.sort == "random") {
                val order = randomOrder(query.path, selection, args, signal)
                val start = offset.coerceAtMost(order.size)
                val ids = order.copyOfRange(start, (start.toLong()+96).coerceAtMost(order.size.toLong()).toInt())
                val photos = randomPage(selection, args, ids, signal)
                return@read PhotoPage(query.path, "", page, order.size, start.toLong()+96 < order.size, 0, false, false,
                    photos, emptyList(), emptyList())
            }
            clearPlaybackOrder()
            val photos = queryPage(selection, args, offset, signal)
            PhotoPage(query.path, "", page, count, photos.size > 96, 0, false, false,
                photos.take(96), emptyList(), emptyList())
        }
    }

    override suspend fun info(path: String): Photo = read { signal ->
        // Accept only an item in this catalog's image collection, never arbitrary content providers.
        val uri = Uri.parse(path)
        require(uri.scheme == "content" && uri.authority == collection.authority &&
            uri.pathSegments.dropLast(1) == collection.pathSegments && uri.lastPathSegment?.toLongOrNull() != null)
        resolver.query(uri, PHOTO_COLUMNS, Bundle(), signal)?.use { cursor ->
            if(cursor.moveToFirst()) photo(cursor) else throw UserIoFailure(UiText(R.string.error_missing))
        } ?: throw UserIoFailure(UiText(R.string.photos_device_error))
    }

    fun folderName(path: String): String? = catalog?.folders?.firstOrNull { it.path == path }?.name

    // Revalidate the accessible MediaStore rows on resume, including changes to
    // Android's selected-photo grant. Keep the viewer, paging window and image
    // loader when nothing changed. A changed catalog is reused without a second
    // scan and never mutates the snapshot used by in-flight page requests.
    suspend fun refreshed(): DevicePhotosService? = read { signal ->
        val previous = catalog ?: return@read null
        val current = scanFolders(signal)
        if(previous.revision.contentEquals(current.revision)) null
        else DevicePhotosService(resolver, random).also { it.catalog = current }
    }

    private fun randomOrder(folder: String, selection: String, args: Array<String>, signal: CancellationSignal): LongArray {
        val generation = playbackGeneration.get()
        playbackOrder?.takeIf {it.folder==folder && it.generation==generation}?.let {return it.ids}
        // Only primitive IDs are retained (8 bytes per image), never a complete
        // Photo catalog. Each metadata page below remains bounded to 96 images.
        val ids = resolver.query(collection, arrayOf(MediaStore.Images.Media._ID), selection, args, PHOTO_ORDER, signal)?.use { cursor ->
            val result = LongArray(cursor.count)
            var count = 0
            while(cursor.moveToNext()) {
                signal.throwIfCanceled()
                result[count++] = cursor.getLong(0)
            }
            if(count==result.size) result else result.copyOf(count)
        } ?: throw UserIoFailure(UiText(R.string.photos_device_error))
        ids.shuffle(random)
        signal.throwIfCanceled()
        if(playbackGeneration.get()==generation) playbackOrder=PlaybackOrder(folder,generation,ids)
        return ids
    }

    private fun randomPage(selection: String, args: Array<String>, ids: LongArray, signal: CancellationSignal): List<Photo> {
        if(ids.isEmpty()) return emptyList()
        val selected = "$selection AND ${MediaStore.Images.Media._ID} IN (${ids.joinToString(",") {"?"}})"
        val selectedArgs = args + ids.map {it.toString()}
        val photos = resolver.query(collection, PHOTO_COLUMNS, selected, selectedArgs, PHOTO_ORDER, signal)?.use { readPhotos(it,signal) }
            ?: throw UserIoFailure(UiText(R.string.photos_device_error))
        val byPath = photos.associateBy {it.path}
        return buildList {ids.forEach {id -> byPath[ContentUris.withAppendedId(collection,id).toString()]?.let(::add)}}
    }

    private fun scanFolders(signal: CancellationSignal): Catalog {
        data class Bucket(val id: String, val volume: String?, val name: String, var count: Int = 0,
            val previews: MutableList<Photo> = mutableListOf())
        val columns = (listOf(MediaStore.Images.Media.BUCKET_ID) + PHOTO_COLUMNS).toMutableList()
        if(Build.VERSION.SDK_INT >= 29) columns += MediaStore.MediaColumns.VOLUME_NAME
        val found = linkedMapOf<String, Bucket>()
        val revision = MessageDigest.getInstance("SHA-256")
        // One streaming metadata pass; retain only counts, two thumbnail IDs per
        // folder and a fixed-size digest, never every image's metadata or ID.
        resolver.query(collection, columns.toTypedArray(), null, null, PHOTO_ORDER, signal)?.use { cursor ->
            while(cursor.moveToNext()) {
                signal.throwIfCanceled()
                for(column in columns.indices) {
                    revision.update(cursor.getString(column).orEmpty().toByteArray(Charsets.UTF_8))
                    revision.update(0.toByte())
                }
                val id = cursor.getString(0) ?: continue
                val volume = if(Build.VERSION.SDK_INT >= 29) cursor.getString(columns.lastIndex) else null
                val key = "${volume.orEmpty()}:$id"
                val bucket = found.getOrPut(key) { Bucket(id, volume,
                    cursor.getString(cursor.getColumnIndexOrThrow(MediaStore.Images.Media.BUCKET_DISPLAY_NAME)).orEmpty().ifBlank { id }) }
                bucket.count++
                if(bucket.previews.size < 2) bucket.previews += Photo(
                    ContentUris.withAppendedId(collection, cursor.getLong(1)).toString(), bucket.name,
                    "image", "image/*", "", "", null, 0, 0, 0)
            }
        } ?: throw UserIoFailure(UiText(R.string.photos_device_error))
        val buckets = mutableMapOf<String, Pair<String, String?>>()
        val folders = found.map { (key, bucket) ->
            buckets[key] = bucket.id to bucket.volume
            PhotoFolder(key, bucket.name, null, bucket.count, false, 0, bucket.previews)
        }.sortedWith(compareBy<PhotoFolder> { it.name.lowercase(java.util.Locale.ROOT) }.thenBy { it.path })
        return Catalog(folders, buckets, revision.digest())
    }

    private fun queryPage(selection: String, args: Array<String>, offset: Int, signal: CancellationSignal): List<Photo> {
        val query = Bundle().apply {
            putString(ContentResolver.QUERY_ARG_SQL_SELECTION, selection)
            putStringArray(ContentResolver.QUERY_ARG_SQL_SELECTION_ARGS, args)
            // Match the folder previews: newest indexed images first, including
            // screenshots/imports with absent or older capture dates. The unique
            // indexed ID also keeps page boundaries deterministic and inexpensive.
            putString(ContentResolver.QUERY_ARG_SQL_SORT_ORDER, PHOTO_ORDER)
            putInt(ContentResolver.QUERY_ARG_LIMIT, 97)
            putInt(ContentResolver.QUERY_ARG_OFFSET, offset)
        }
        val cursor = resolver.query(collection, PHOTO_COLUMNS, query, signal)
            ?: throw UserIoFailure(UiText(R.string.photos_device_error))
        val honored = cursor.extras.getStringArray(ContentResolver.EXTRA_HONORED_ARGS).orEmpty()
        if(ContentResolver.QUERY_ARG_LIMIT in honored && ContentResolver.QUERY_ARG_OFFSET in honored) {
            return cursor.use { readPhotos(it, signal) }
        }
        // Older/OEM providers may ignore paging arguments. Never mistake a limited
        // first page for a later page: requery without either argument and seek.
        cursor.close()
        query.remove(ContentResolver.QUERY_ARG_LIMIT)
        query.remove(ContentResolver.QUERY_ARG_OFFSET)
        return resolver.query(collection, PHOTO_COLUMNS, query, signal)?.use {
            it.moveToPosition(offset - 1)
            readPhotos(it, signal)
        } ?: throw UserIoFailure(UiText(R.string.photos_device_error))
    }

    private fun readPhotos(cursor: Cursor, signal: CancellationSignal): List<Photo> = buildList {
        while(size < 97 && cursor.moveToNext()) {
            signal.throwIfCanceled()
            add(photo(cursor))
        }
    }

    private fun photo(cursor: Cursor): Photo {
        val modified = cursor.getLong(3)
        val captured = cursor.getLong(4)
        return Photo(ContentUris.withAppendedId(collection, cursor.getLong(0)).toString(),
            cursor.getString(1).orEmpty(), "image", cursor.getString(2) ?: "image/*",
            "$modified:${cursor.getLong(5)}", date(modified * 1000), captured.takeIf { it > 0 }?.let(::date),
            cursor.getLong(5), cursor.getInt(6), cursor.getInt(7),folderName=cursor.getString(8).orEmpty())
    }

    private suspend fun <T> read(block: (CancellationSignal) -> T): T = coroutineScope {
        val signal = CancellationSignal()
        val cancellation = launch(start=CoroutineStart.UNDISPATCHED) {
            try { awaitCancellation() } finally { signal.cancel() }
        }
        try { withContext(Dispatchers.IO) { block(signal) } }
        catch(e: CancellationException) { throw e }
        catch(e: Exception) {
            ensureActive()
            if(e is UserIoFailure) throw e
            throw UserIoFailure(UiText(if(e is SecurityException) R.string.photos_device_permission else R.string.photos_device_error))
        } finally { cancellation.cancel() }
    }

    companion object {
        private val PHOTO_ORDER = "${MediaStore.Images.Media._ID} DESC"
        val SESSION = PhotoSession("device", false, 240, 240, 1280, 2048, 5, 8, frameRandomSort=true)
        private val PHOTO_COLUMNS = arrayOf(MediaStore.Images.Media._ID, MediaStore.Images.Media.DISPLAY_NAME,
            MediaStore.Images.Media.MIME_TYPE, MediaStore.Images.Media.DATE_MODIFIED, MediaStore.Images.Media.DATE_TAKEN,
            MediaStore.Images.Media.SIZE, MediaStore.Images.Media.WIDTH, MediaStore.Images.Media.HEIGHT, MediaStore.Images.Media.BUCKET_DISPLAY_NAME)
        private fun date(milliseconds: Long) = Instant.ofEpochMilli(milliseconds).atZone(ZoneId.systemDefault()).toOffsetDateTime().toString()
    }
}
