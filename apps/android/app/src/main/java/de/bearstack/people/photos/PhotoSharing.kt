package de.bearstack.people.photos

import android.content.ClipData
import android.content.Context
import android.content.Intent
import androidx.core.net.toUri
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
    private val cache = PhotoShareCache(File(this.context.cacheDir, "photo-sharing"), maxFiles=MAX_PHOTO_SELECTION)

    suspend fun share(photo: Photo, launch: (Intent) -> Unit, progress: (Long, Long) -> Unit = { _, _ -> }) =
        share(listOf(photo), launch, progress)

    suspend fun share(photos: List<Photo>, launch: (Intent) -> Unit, progress: (Long, Long) -> Unit = { _, _ -> }) = lock.withLock {
        require(photos.isNotEmpty() && photos.size <= MAX_PHOTO_SELECTION)
        require(photos.all { it.type in setOf("image", "video", "audio") })
        val temporary = mutableSetOf<File>()
        var launched = false
        try {
            val uris = withContext(Dispatchers.IO) { photos.map { photo ->
                ensureActive()
                if(service is DevicePhotosService) {
                    // Recheck current access and accept only this catalog's MediaStore image URIs.
                    service.info(photo.path).path.toUri()
                } else {
                    if(photo.bytes > cache.maxFileBytes) throw UserIoFailure(UiText(R.string.photos_share_too_large))
                    val extension = MimeTypeMap.getSingleton().getExtensionFromMimeType(photo.mime) ?: "img"
                    val reservation = photo.bytes.takeIf {it>0} ?: cache.maxFileBytes
                    val file = cache.create(extension, temporary, reservation).also { temporary += it }
                    service.download(photo, { cache.output(file) }, progress)
                    ensureActive()
                    FileProvider.getUriForFile(context, "${context.packageName}.photo-sharing", file, photo.name)
                }
            } }
            withContext(Dispatchers.Main.immediate) {
                ensureActive()
                launch(Intent.createChooser(photoShareIntent(photos, uris), null))
                launched = true
            }
        } finally {
            // Keep successful shares available while the receiving app imports them, even
            // after the viewer/session closes. Failed and cancelled transfers leave no file.
            if(!launched) withContext(NonCancellable + Dispatchers.IO) { temporary.forEach {it.delete()} }
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

internal fun photoShareIntent(photos: List<Photo>, uris: List<Uri>): Intent {
    require(photos.isNotEmpty() && photos.size==uris.size)
    if(photos.size==1) return photoShareIntent(photos.single(),uris.single())
    return Intent(Intent.ACTION_SEND_MULTIPLE).apply {
        val types=photos.map {it.mime}.distinct()
        val families=types.map {it.substringBefore('/')}.distinct()
        type=if(types.size==1) types.single() else if(families.size==1) "${families.single()}/*" else "*/*"
        putParcelableArrayListExtra(Intent.EXTRA_STREAM, ArrayList(uris))
        clipData=ClipData.newRawUri(photos.first().name,uris.first()).apply {
            uris.drop(1).forEach {addItem(ClipData.Item(it))}
        }
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
    }
}

/** Flat, private directory; bounded disk use without decoding or buffering the original. */
internal class PhotoShareCache(private val directory: File, val maxFileBytes: Long = 256L * 1024 * 1024,
    private val maxBytes: Long = 512L * 1024 * 1024, private val maxFiles: Int = 8,
    private val maxAgeMillis: Long = 24L * 60 * 60 * 1000) {
    fun create(extension: String, protected: Set<File> = emptySet(), reserveBytes: Long = maxFileBytes): File {
        check(directory.isDirectory || directory.mkdirs())
        val now = System.currentTimeMillis()
        val files = directory.listFiles()?.filter { it.isFile }?.sortedByDescending { it.lastModified() }.orEmpty()
        var retained = protected.size
        var bytes = protected.sumOf {it.length()}
        val reservation=reserveBytes.coerceIn(0,maxFileBytes)
        if(retained >= maxFiles || bytes > maxBytes-reservation) throw UserIoFailure(UiText(R.string.photos_share_too_large))
        for(file in files.filterNot {it in protected}) {
            if(now - file.lastModified() >= maxAgeMillis || retained >= maxFiles - 1 ||
                file.length() > maxBytes - reservation - bytes) {
                check(file.delete())
            } else { retained++; bytes += file.length() }
        }
        return File.createTempFile("photo-", ".${extension.filter { it.isLetterOrDigit() }.take(16)}", directory)
    }

    fun output(file: File): OutputStream {
        val allowed=minOf(maxFileBytes,maxBytes-directory.listFiles().orEmpty().filter {it!=file}.sumOf {it.length()})
        return object : FilterOutputStream(file.outputStream()) {
        private var bytes = 0L
        private fun reserve(count: Int) {
            if(count.toLong() > allowed - bytes) throw UserIoFailure(UiText(R.string.photos_share_too_large))
            bytes += count
        }
        override fun write(value: Int) { reserve(1); out.write(value) }
        override fun write(buffer: ByteArray, offset: Int, length: Int) { reserve(length); out.write(buffer, offset, length) }
    }
    }
}
