package de.bearstack.people.photos

import de.bearstack.people.text.*
import de.bearstack.people.R
import android.content.Context
import android.net.Uri
import android.provider.DocumentsContract
import de.bearstack.people.data.remote.Photo
import de.bearstack.people.data.remote.PhotosService
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import java.io.IOException

data class PhotoDownloadState(val name: String = "", val active: Boolean = false, val complete: Boolean = false,
    val received: Long = 0, val total: Long = -1, val error: UiText? = null)

// The system picker grants access to one destination. No storage permission or
// shared download manager receives the BearStack credentials.
class PhotoDownloads(context: Context, private val scope: CoroutineScope, private val service: PhotosService) {
    private val context=context.applicationContext
    private val resolver=this.context.contentResolver
    private val mutable=MutableStateFlow(PhotoDownloadState())
    val state=mutable.asStateFlow()
    var pending: Photo? = null
    private var task: Job? = null
    fun save(photo: Photo, uri: Uri) {
        if(state.value.active || !scope.isActive) return
        mutable.value=PhotoDownloadState(name=photo.name,active=true)
        task=scope.launch {
            try {
                val bytes=service.download(photo,{resolver.openOutputStream(uri,"w") ?: throw UserIoFailure(UiText(R.string.error_download_target))}) { received,total ->
                    mutable.update {it.copy(received=received,total=total)}
                }
                mutable.update {it.copy(active=false,complete=true,received=bytes)}
            } catch(e: CancellationException) {
                try {removeIncomplete(uri)} finally {mutable.value=PhotoDownloadState()}
                throw e
            } catch(e: Exception) {
                try {removeIncomplete(uri)} finally {
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
