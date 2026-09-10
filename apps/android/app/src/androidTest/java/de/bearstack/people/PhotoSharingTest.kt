package de.bearstack.people

import android.content.ActivityNotFoundException
import android.content.ContentValues
import android.content.Intent
import android.net.Uri
import android.provider.MediaStore
import android.provider.OpenableColumns
import androidx.core.content.FileProvider
import androidx.test.filters.SdkSuppress
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.*
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Test
import java.io.File
import java.io.IOException
import java.io.OutputStream

internal val shareTestPhoto = Photo("private/first.jpg", "first.jpg", "image", "image/jpeg", "1",
    "2026-09-09T10:00:00Z", null, 3, 10, 10)

internal open class ShareTestService : PhotosService {
    val entered = CompletableDeferred<Unit>()
    var mode = "success"
    var selected: Photo? = null
    override suspend fun session(): PhotoSession = error("unused")
    override suspend fun browse(query: PhotoQuery, page: Int, section: String): PhotoPage = error("unused")
    override suspend fun info(path: String): Photo = error("unused")
    override suspend fun blog(path: String): PhotoBlog = error("unused")
    override fun thumbnail(photo: Photo, size: Int): String = error("unused")
    override fun original(photo: Photo): String = error("unused")
    override suspend fun download(photo: Photo, destination: () -> OutputStream, progress: (Long, Long) -> Unit): Long {
        selected = photo
        return destination().use {
            it.write(byteArrayOf(1, 2, 3))
            entered.complete(Unit)
            if(mode == "failure") throw IOException("secret server details")
            if(mode == "cancel") awaitCancellation()
            3L
        }
    }
}

class PhotoSharingTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private val directory get() = File(context.cacheDir, "photo-sharing")
    private fun files() = directory.listFiles()?.toSet().orEmpty()

    @Suppress("DEPRECATION")
    @Test fun originalIsReadableFromChooserWithNameMimeAndOnlyTemporaryReadAccess() = runBlocking {
        val before = files()
        val service = ShareTestService()
        try {
            var launched = false
            PhotoSharing(context, service).share(shareTestPhoto, { chooser ->
                launched = true
                assertEquals(Intent.ACTION_CHOOSER, chooser.action)
                val intent = chooser.getParcelableExtra<Intent>(Intent.EXTRA_INTENT)!!
                assertEquals(Intent.ACTION_SEND, intent.action)
                assertEquals("image/jpeg", intent.type)
                assertEquals(Intent.FLAG_GRANT_READ_URI_PERMISSION, intent.flags)
                assertNull(intent.getStringExtra(Intent.EXTRA_TEXT))
                val uri = intent.getParcelableExtra<Uri>(Intent.EXTRA_STREAM)!!
                assertEquals("content", uri.scheme)
                assertEquals("${context.packageName}.photo-sharing", uri.authority)
                assertEquals(uri, intent.clipData!!.getItemAt(0).uri)
                assertEquals(uri, chooser.clipData!!.getItemAt(0).uri)
                assertEquals(Intent.FLAG_GRANT_READ_URI_PERMISSION, chooser.flags and Intent.FLAG_GRANT_READ_URI_PERMISSION)
                assertEquals("image/jpeg", context.contentResolver.getType(uri))
                context.contentResolver.openInputStream(uri)!!.use { assertArrayEquals(byteArrayOf(1, 2, 3), it.readBytes()) }
                context.contentResolver.query(uri, null, null, null, null)!!.use {
                    assertTrue(it.moveToFirst())
                    assertEquals("first.jpg", it.getString(it.getColumnIndexOrThrow(OpenableColumns.DISPLAY_NAME)))
                    assertEquals(3L, it.getLong(it.getColumnIndexOrThrow(OpenableColumns.SIZE)))
                }
            })
            assertTrue(launched)
            assertEquals(shareTestPhoto, service.selected)
            assertEquals(1, (files() - before).size)
        } finally { (files() - before).forEach { it.delete() } }
    }

    @Test fun failedCancelledAndUnlaunchableSharesRemoveTheirFiles() = runBlocking {
        for(mode in listOf("failure", "cancel", "unlaunchable")) {
            val before = files()
            val service = ShareTestService().apply { this.mode = mode }
            val task = launch {
                try {
                    PhotoSharing(context, service).share(shareTestPhoto, {
                        assertEquals("unlaunchable", mode)
                        throw ActivityNotFoundException()
                    })
                    fail("unsuccessful share completed")
                } catch(_: IOException) { assertEquals("failure", mode) }
                catch(_: ActivityNotFoundException) { assertEquals("unlaunchable", mode) }
            }
            withTimeout(5000) { service.entered.await() }
            if(mode == "cancel") task.cancel()
            withTimeout(5000) { task.join() }
            assertEquals("$mode left a temporary file", before, files())
        }
    }

    @Test fun oversizedOriginalIsRejectedWithoutDownloading() = runBlocking {
        val service = ShareTestService()
        try {
            PhotoSharing(context, service).share(shareTestPhoto.copy(bytes = 256L * 1024 * 1024 + 1), { fail("chooser opened") })
            fail("oversized image accepted")
        } catch(_: IOException) { assertNull(service.selected) }
    }

    @Test fun providerCannotExposeOtherPrivateFiles() {
        val private = File(context.cacheDir, "not-shared.jpg").apply { writeText("private") }
        try {
            try {
                FileProvider.getUriForFile(context, "${context.packageName}.photo-sharing", private)
                fail("file outside sharing directory exposed")
            } catch(_: IllegalArgumentException) { }
            val provider = context.packageManager.resolveContentProvider("${context.packageName}.photo-sharing", 0)!!
            assertFalse(provider.exported)
            assertTrue(provider.grantUriPermissions)
        } finally { private.delete() }
    }

    @Suppress("DEPRECATION")
    @SdkSuppress(minSdkVersion = 29)
    @Test fun localPhotoUsesItsMediaStoreUriWithoutCopyAndMissingPhotoFails() = runBlocking {
        val resolver = context.contentResolver
        val uri = resolver.insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, ContentValues().apply {
            put(MediaStore.Images.Media.DISPLAY_NAME, "share-local.jpg")
            put(MediaStore.Images.Media.MIME_TYPE, "image/jpeg")
            put(MediaStore.Images.Media.RELATIVE_PATH, "Pictures/BearStackShareTest")
        })!!
        val before = files()
        var removed = false
        try {
            resolver.openOutputStream(uri)!!.use { it.write(byteArrayOf(1, 2, 3)) }
            val service = DevicePhotosService(resolver)
            val photo = service.info(uri.toString())
            PhotoSharing(context, service).share(photo, { chooser ->
                val send = chooser.getParcelableExtra<Intent>(Intent.EXTRA_INTENT)!!
                assertEquals(uri, send.getParcelableExtra<Uri>(Intent.EXTRA_STREAM))
                assertEquals(Intent.FLAG_GRANT_READ_URI_PERMISSION, send.flags)
            })
            assertEquals(before, files())
            resolver.delete(uri, null, null)
            removed = true
            try {
                PhotoSharing(context, service).share(photo, { fail("missing local photo shared") })
                fail("deleted local photo accepted")
            } catch(_: IOException) { }
            catch(_: SecurityException) { } // MediaStore may revoke the deleted item's grant immediately.
        } finally { if(!removed) resolver.delete(uri, null, null) }
    }
}
