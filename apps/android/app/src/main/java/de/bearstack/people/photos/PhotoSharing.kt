package de.bearstack.people.photos

import android.content.ClipData
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.webkit.MimeTypeMap
import androidx.core.content.FileProvider
import de.bearstack.people.R
import de.bearstack.people.data.remote.Photo
import de.bearstack.people.data.remote.PhotosService
import de.bearstack.people.text.UiText
import de.bearstack.people.text.UserIoFailure
import kotlinx.coroutines.*
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.io.File
import java.io.FilterOutputStream
import java.io.OutputStream

class PhotoShareProvider : FileProvider()

/** Only completed originals enter the system chooser; credentials never leave the HTTP client. */
internal class PhotoSharing(context: Context, private val service: PhotosService) {
    private val context = context.applicationContext
    private val cache = PhotoShareCache(File(this.context.cacheDir, "photo-sharing"))

    suspend fun share(photo: Photo, launch: (Intent) -> Unit, progress: (Long, Long) -> Unit = { _, _ -> }) = lock.withLock {
        require(photo.type == "image" && photo.mime.startsWith("image/"))
        var temporary: File? = null
        var launched = false
        try {
            val uri = withContext(Dispatchers.IO) {
                if(service is DevicePhotosService) {
                    // Recheck current access and accept only this catalog's MediaStore image URIs.
                    Uri.parse(service.info(photo.path).path)
                } else {
                    if(photo.bytes > cache.maxFileBytes) throw UserIoFailure(UiText(R.string.photos_share_too_large))
                    val extension = MimeTypeMap.getSingleton().getExtensionFromMimeType(photo.mime) ?: "img"
                    val file = cache.create(extension).also { temporary = it }
                    service.download(photo, { cache.output(file) }, progress)
                    ensureActive()
                    FileProvider.getUriForFile(context, "${context.packageName}.photo-sharing", file, photo.name)
                }
            }
            withContext(Dispatchers.Main.immediate) {
                ensureActive()
                launch(Intent.createChooser(photoShareIntent(photo, uri), null))
                launched = true
            }
        } finally {
            // Keep successful shares available while the receiving app imports them, even
            // after the viewer/session closes. Failed and cancelled transfers leave no file.
            if(!launched) withContext(NonCancellable + Dispatchers.IO) { temporary?.delete() }
        }
    }

    private companion object { val lock = Mutex() }
}

internal fun photoShareIntent(photo: Photo, uri: Uri) = Intent(Intent.ACTION_SEND).apply {
    type = photo.mime
    putExtra(Intent.EXTRA_STREAM, uri)
    clipData = ClipData.newRawUri(photo.name, uri)
    addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
}

/** Flat, private directory; bounded disk use without decoding or buffering the original. */
internal class PhotoShareCache(private val directory: File, val maxFileBytes: Long = 256L * 1024 * 1024,
    private val maxBytes: Long = 512L * 1024 * 1024, private val maxFiles: Int = 8,
    private val maxAgeMillis: Long = 24L * 60 * 60 * 1000) {
    fun create(extension: String): File {
        check(directory.isDirectory || directory.mkdirs())
        val now = System.currentTimeMillis()
        val files = directory.listFiles()?.filter { it.isFile }?.sortedByDescending { it.lastModified() }.orEmpty()
        var retained = 0
        var bytes = 0L
        for(file in files) {
            if(now - file.lastModified() >= maxAgeMillis || retained >= maxFiles - 1 ||
                file.length() > maxBytes - maxFileBytes - bytes) {
                check(file.delete())
            } else { retained++; bytes += file.length() }
        }
        return File.createTempFile("photo-", ".${extension.filter { it.isLetterOrDigit() }.take(16)}", directory)
    }

    fun output(file: File): OutputStream = object : FilterOutputStream(file.outputStream()) {
        private var bytes = 0L
        private fun reserve(count: Int) {
            if(count.toLong() > maxFileBytes - bytes) throw UserIoFailure(UiText(R.string.photos_share_too_large))
            bytes += count
        }
        override fun write(value: Int) { reserve(1); out.write(value) }
        override fun write(buffer: ByteArray, offset: Int, length: Int) { reserve(length); out.write(buffer, offset, length) }
    }
}
