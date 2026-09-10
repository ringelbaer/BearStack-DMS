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

/** Read-only, foreground-scoped MediaStore catalog. No server client or filesystem traversal. */
internal class DevicePhotosService(private val resolver: ContentResolver) : PhotosService {
    private val collection = MediaStore.Images.Media.EXTERNAL_CONTENT_URI
    private var catalog: List<PhotoFolder>? = null
    private val buckets = mutableMapOf<String, Pair<String, String?>>()

    override suspend fun session() = SESSION
    override fun thumbnail(photo: Photo, size: Int) = photo.path
    override fun original(photo: Photo) = photo.path
    override suspend fun blog(path: String): PhotoBlog = throw UnsupportedOperationException()

    override suspend fun browse(query: PhotoQuery, page: Int, section: String): PhotoPage = read { signal ->
        require(page > 0)
        val folders = catalog ?: scanFolders(signal).also { catalog = it }
        if(query.path.isEmpty()) {
            val start = ((page - 1).toLong() * 24).coerceAtMost(folders.size.toLong()).toInt()
            PhotoPage("", "", page, 0, false, folders.size, start + 24 < folders.size, false,
                emptyList(), folders.subList(start, (start + 24).coerceAtMost(folders.size)), emptyList())
        } else {
            val bucket = buckets[query.path] ?: throw UserIoFailure(UiText(R.string.error_missing))
            val count = folders.first { it.path == query.path }.count
            val selection = MediaStore.Images.Media.BUCKET_ID + " = ?" +
                if(bucket.second != null) " AND ${MediaStore.MediaColumns.VOLUME_NAME} = ?" else ""
            val args = listOfNotNull(bucket.first, bucket.second).toTypedArray()
            val offset = Math.multiplyExact(page - 1, 96)
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

    fun folderName(path: String): String? = catalog?.firstOrNull { it.path == path }?.name

    private fun scanFolders(signal: CancellationSignal): List<PhotoFolder> {
        data class Bucket(val id: String, val volume: String?, val name: String, var count: Int = 0,
            val previews: MutableList<Photo> = mutableListOf())
        val columns = mutableListOf(MediaStore.Images.Media._ID, MediaStore.Images.Media.BUCKET_ID,
            MediaStore.Images.Media.BUCKET_DISPLAY_NAME)
        if(Build.VERSION.SDK_INT >= 29) columns += MediaStore.MediaColumns.VOLUME_NAME
        val found = linkedMapOf<String, Bucket>()
        // One streaming metadata pass; retain only counts and two thumbnail IDs per folder.
        resolver.query(collection, columns.toTypedArray(), null, null, "${MediaStore.Images.Media._ID} DESC", signal)?.use { cursor ->
            while(cursor.moveToNext()) {
                signal.throwIfCanceled()
                val id = cursor.getString(1) ?: continue
                val volume = if(Build.VERSION.SDK_INT >= 29) cursor.getString(3) else null
                val key = "${volume.orEmpty()}:$id"
                val bucket = found.getOrPut(key) { Bucket(id, volume, cursor.getString(2).orEmpty().ifBlank { id }) }
                bucket.count++
                if(bucket.previews.size < 2) bucket.previews += Photo(
                    ContentUris.withAppendedId(collection, cursor.getLong(0)).toString(), bucket.name,
                    "image", "image/*", "", "", null, 0, 0, 0)
            }
        } ?: throw UserIoFailure(UiText(R.string.photos_device_error))
        buckets.clear()
        return found.map { (key, bucket) ->
            buckets[key] = bucket.id to bucket.volume
            PhotoFolder(key, bucket.name, null, bucket.count, false, 0, bucket.previews)
        }.sortedWith(compareBy<PhotoFolder> { it.name.lowercase(java.util.Locale.ROOT) }.thenBy { it.path })
    }

    private fun queryPage(selection: String, args: Array<String>, offset: Int, signal: CancellationSignal): List<Photo> {
        val query = Bundle().apply {
            putString(ContentResolver.QUERY_ARG_SQL_SELECTION, selection)
            putStringArray(ContentResolver.QUERY_ARG_SQL_SELECTION_ARGS, args)
            putString(ContentResolver.QUERY_ARG_SQL_SORT_ORDER,
                "${MediaStore.Images.Media.DATE_TAKEN} DESC, ${MediaStore.Images.Media._ID} DESC")
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
            cursor.getLong(5), cursor.getInt(6), cursor.getInt(7))
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
        val SESSION = PhotoSession("device", false, 240, 240, 1280, 2048, 5, 8)
        private val PHOTO_COLUMNS = arrayOf(MediaStore.Images.Media._ID, MediaStore.Images.Media.DISPLAY_NAME,
            MediaStore.Images.Media.MIME_TYPE, MediaStore.Images.Media.DATE_MODIFIED, MediaStore.Images.Media.DATE_TAKEN,
            MediaStore.Images.Media.SIZE, MediaStore.Images.Media.WIDTH, MediaStore.Images.Media.HEIGHT)
        private fun date(milliseconds: Long) = Instant.ofEpochMilli(milliseconds).atZone(ZoneId.systemDefault()).toOffsetDateTime().toString()
    }
}
