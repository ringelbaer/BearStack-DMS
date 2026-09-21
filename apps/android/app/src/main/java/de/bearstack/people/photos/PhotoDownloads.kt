package de.bearstack.people.photos

import de.bearstack.people.text.*
import de.bearstack.people.R
import android.content.Context
import android.net.Uri
import android.provider.DocumentsContract
import android.os.Build
import android.view.accessibility.AccessibilityManager
import de.bearstack.people.data.remote.Photo
import de.bearstack.people.data.remote.PhotosService
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import java.io.IOException

data class PhotoDownloadState(val name: String = "", val active: Boolean = false, val complete: Boolean = false,
    val received: Long = 0, val total: Long = -1, val error: UiText? = null,
    val savedCount: Int = 0, val requestedCount: Int = 1)

// The system picker grants access to one destination. No storage permission or
// shared download manager receives the BearStack credentials.
class PhotoDownloads(context: Context, private val scope: CoroutineScope, private val service: PhotosService) {
    private val context=context.applicationContext
    private val resolver=this.context.contentResolver
    private val mutable=MutableStateFlow(PhotoDownloadState())
    val state=mutable.asStateFlow()
    var pending: Photo? = null
    private var task: Job? = null
    private var statusTask: Job? = null
    fun save(photo: Photo, uri: Uri) = saveBatch(listOf(photo)) { uri }
    fun saveAll(photos: List<Photo>, tree: Uri) = saveBatch(photos) { photo ->
        val parent = DocumentsContract.buildDocumentUriUsingTree(tree, DocumentsContract.getTreeDocumentId(tree))
        DocumentsContract.createDocument(resolver, parent, photo.mime, photo.name)
            ?: throw UserIoFailure(UiText(R.string.error_download_target))
    }
    internal fun saveBatch(photos: List<Photo>, destination: (Photo) -> Uri) {
        if(state.value.active || !scope.isActive) return
        require(photos.isNotEmpty() && photos.size <= MAX_PHOTO_SELECTION)
        val selected = photos.toList()
        statusTask?.cancel()
        mutable.value=PhotoDownloadState(name=selected.first().name,active=true,requestedCount=selected.size)
        task=scope.launch {
            var incomplete: Uri? = null
            try {
                for(photo in selected) {
                    ensureActive()
                    mutable.update {it.copy(name=photo.name,received=0,total=-1)}
                    withContext(Dispatchers.IO) { incomplete=destination(photo) }
                    val uri=checkNotNull(incomplete)
                    val bytes=service.download(photo,{resolver.openOutputStream(uri,"w") ?: throw UserIoFailure(UiText(R.string.error_download_target))}) { received,total ->
                        mutable.update {it.copy(received=received,total=total)}
                    }
                    incomplete=null
                    mutable.update {it.copy(received=bytes,savedCount=it.savedCount+1)}
                }
                val completed = state.value.copy(active=false,complete=true)
                mutable.value = completed
                statusTask = scope.launch {
                    val accessibility = context.getSystemService(AccessibilityManager::class.java)
                    val timeout = if(Build.VERSION.SDK_INT >= 29)
                        accessibility?.getRecommendedTimeoutMillis(5000, AccessibilityManager.FLAG_CONTENT_TEXT) ?: 5000
                        else 5000
                    delay(timeout.toLong())
                    mutable.compareAndSet(completed, PhotoDownloadState())
                }
            } catch(e: CancellationException) {
                try {incomplete?.let {removeIncomplete(it)}} finally {mutable.value=PhotoDownloadState()}
                throw e
            } catch(e: Exception) {
                try {incomplete?.let {removeIncomplete(it)}} finally {
                    mutable.update {it.copy(active=false,error=failureText(e))}
                }
            }
        }
    }
    private suspend fun removeIncomplete(uri: Uri) = withContext(NonCancellable+Dispatchers.IO) {
        runCatching {
            if(DocumentsContract.isDocumentUri(context,uri)) DocumentsContract.deleteDocument(resolver,uri)
            else resolver.delete(uri,null,null)
        }
        Unit
    }
    fun cancel() {task?.cancel()}
}
