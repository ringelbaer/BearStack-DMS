package de.bearstack.people

import android.content.*
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
import android.os.Bundle
import android.os.CancellationSignal
import android.provider.MediaStore
import androidx.test.filters.SdkSuppress
import de.bearstack.people.data.remote.PhotoQuery
import de.bearstack.people.photos.DevicePhotosService
import de.bearstack.people.text.DescribedFailure
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

@SdkSuppress(minSdkVersion=29)
class DevicePhotosServiceTest {
    private data class Entry(val id: Long, val bucket: String, val name: String, val volume: String = "external_primary")
    private class Provider(val entries: List<Entry>, val paging: Boolean = true) : ContentProvider() {
        var folderScans = 0
        var closedCursors = 0
        var openedCursors = 0
        var fail = false
        var block = false
        val entered = CountDownLatch(1)
        val cancelled = CountDownLatch(1)
        override fun onCreate() = true
        override fun getType(uri: Uri) = "image/jpeg"
        override fun insert(uri: Uri, values: ContentValues?): Uri? = error("must stay read-only")
        override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?): Int = error("must stay read-only")
        override fun update(uri: Uri, values: ContentValues?, selection: String?, selectionArgs: Array<out String>?): Int = error("must stay read-only")
        override fun query(uri: Uri, projection: Array<out String>?, selection: String?, selectionArgs: Array<out String>?, sortOrder: String?): Cursor =
            query(uri, projection, Bundle(), null)
        override fun query(uri: Uri, projection: Array<out String>?, queryArgs: Bundle?, cancellationSignal: CancellationSignal?): Cursor {
            if(fail) throw SecurityException("private provider details")
            if(block) {
                cancellationSignal!!.setOnCancelListener { cancelled.countDown() }
                entered.countDown()
                check(cancelled.await(5, TimeUnit.SECONDS))
                cancellationSignal.throwIfCanceled()
            }
            val columns = projection!!
            val folders = MediaStore.Images.Media.BUCKET_ID in columns
            if(folders) folderScans++
            val arguments = queryArgs?.getStringArray(ContentResolver.QUERY_ARG_SQL_SELECTION_ARGS)
            var matching = entries.filter { entry ->
                (arguments == null || entry.bucket == arguments[0] && entry.volume == arguments.getOrNull(1)) &&
                    (uri.lastPathSegment == "media" || uri.lastPathSegment == entry.id.toString())
            }.sortedByDescending { it.id }
            val limited = paging && queryArgs?.containsKey(ContentResolver.QUERY_ARG_LIMIT) == true
            if(limited) matching = matching.drop(queryArgs!!.getInt(ContentResolver.QUERY_ARG_OFFSET))
                .take(queryArgs.getInt(ContentResolver.QUERY_ARG_LIMIT))
            openedCursors++
            return object : MatrixCursor(columns) {
                override fun getExtras() = Bundle().apply {
                    if(limited) putStringArray(ContentResolver.EXTRA_HONORED_ARGS,
                        arrayOf(ContentResolver.QUERY_ARG_LIMIT, ContentResolver.QUERY_ARG_OFFSET))
                }
                override fun close() { super.close(); closedCursors++ }
            }.apply {
                matching.forEach { entry -> addRow(columns.map<String, Any> { column -> when(column) {
                    MediaStore.Images.Media._ID -> entry.id
                    MediaStore.Images.Media.BUCKET_ID -> entry.bucket
                    MediaStore.Images.Media.BUCKET_DISPLAY_NAME -> entry.name
                    MediaStore.MediaColumns.VOLUME_NAME -> entry.volume
                    MediaStore.Images.Media.DISPLAY_NAME -> "${entry.id}.jpg"
                    MediaStore.Images.Media.MIME_TYPE -> "image/jpeg"
                    MediaStore.Images.Media.DATE_MODIFIED -> 1_700_000_000L + entry.id
                    MediaStore.Images.Media.DATE_TAKEN -> 1_700_000_000_000L + entry.id * 1000
                    MediaStore.Images.Media.SIZE -> 2048L
                    MediaStore.Images.Media.WIDTH -> 400
                    MediaStore.Images.Media.HEIGHT -> 300
                    else -> error(column)
                } }.toTypedArray()) }
            }
        }
    }
    @Test fun foldersSeparateVolumesPagePast24AndRetainOnlyTwoPreviews() = runBlocking {
        val entries = (1L..300).map { Entry(it, "1", "Camera") } +
            (2L..28).map { Entry(300+it, it.toString(), "Folder $it") } + Entry(400, "1", "Camera", "sd_card")
        val provider = Provider(entries)
        val service = DevicePhotosService(ContentResolver.wrap(provider))
        val first = service.browse(PhotoQuery())
        val second = service.browse(PhotoQuery(), 2, "folders")
        assertEquals(29, first.folderTotal)
        assertEquals(24, first.folders.size)
        assertEquals(5, second.folders.size)
        assertTrue(first.folderHasNext)
        assertFalse(second.folderHasNext)
        val cameras = (first.folders+second.folders).filter { it.name == "Camera" }
        assertEquals(2, cameras.size)
        assertEquals(2, cameras.map { it.path }.distinct().size)
        assertEquals(listOf(1, 300), cameras.map { it.count }.sorted())
        assertTrue(first.folders.all { it.previews.size <= 2 })
        assertEquals(1, provider.folderScans)
        assertEquals(provider.openedCursors, provider.closedCursors)
    }
    @Test fun photoPagesAndInfoWorkWithNativePaging() = pages(true)
    @Test fun olderProvidersWithoutPagingNeverRepeatTheFirstPage() = pages(false)
    private fun pages(paging: Boolean) = runBlocking {
        val provider = Provider((1L..300).map { Entry(it, "1", "Camera") }, paging)
        val service = DevicePhotosService(ContentResolver.wrap(provider))
        val folder = service.browse(PhotoQuery()).folders.single()
        val pages = (1..4).map { service.browse(PhotoQuery(path=folder.path), it, "media") }
        assertEquals(listOf(96, 96, 96, 12), pages.map { it.media.size })
        assertEquals(listOf(true, true, true, false), pages.map { it.hasNext })
        assertEquals(300, pages.flatMap { it.media }.map { it.path }.distinct().size)
        assertEquals(pages[0].media, service.browse(PhotoQuery(path=folder.path), 1, "media").media)
        val photo = pages[0].media.first()
        val info = service.info(photo.path)
        assertEquals("300.jpg", info.name)
        assertEquals(400, info.width)
        assertEquals(300, info.height)
        assertEquals(2048L, info.bytes)
        assertNotNull(info.captured)
        assertTrue(service.thumbnail(photo, 240).startsWith("content://media/external/images/media/"))
        assertEquals(1, provider.folderScans)
        assertEquals(provider.openedCursors, provider.closedCursors)
    }
    @Test fun emptyPermissionDeniedAndForeignUrisAreHandledWithoutTechnicalDetails() = runBlocking {
        val provider = Provider(emptyList())
        val service = DevicePhotosService(ContentResolver.wrap(provider))
        assertTrue(service.browse(PhotoQuery()).folders.isEmpty())
        provider.fail = true
        try { DevicePhotosService(ContentResolver.wrap(provider)).browse(PhotoQuery()); fail("expected denied access") }
        catch(e: Exception) { assertEquals(R.string.photos_device_permission, (e as DescribedFailure).userText.resource) }
        provider.fail = false
        try { service.info("content://other.provider/private/1"); fail("expected rejected URI") }
        catch(e: Exception) { assertEquals(R.string.photos_device_error, (e as DescribedFailure).userText.resource) }
    }
    @Test fun leavingTheCatalogCancelsAProviderQuery() = runBlocking {
        val provider = Provider(emptyList()).apply { block = true }
        val task = launch { DevicePhotosService(ContentResolver.wrap(provider)).browse(PhotoQuery()) }
        withContext(Dispatchers.IO) { assertTrue(provider.entered.await(5, TimeUnit.SECONDS)) }
        withTimeout(3000) { task.cancelAndJoin() }
        assertEquals(0L, provider.cancelled.count)
    }
}
