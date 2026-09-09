package de.bearstack.people.photos

import de.bearstack.people.text.*
import de.bearstack.people.data.remote.*
import de.bearstack.people.people.WifiOriginalPreloader
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*

data class PhotosState(val query: PhotoQuery = PhotoQuery(recursive=true), val tab: Int = 0, val loading: Boolean = false,
    val error: UiText? = null, val media: List<Photo> = emptyList(), val folders: List<PhotoFolder> = emptyList(),
    val blogs: List<PhotoBlog> = emptyList(), val total: Int = 0, val hasNext: Boolean = false,
    val folderHasNext: Boolean = false, val blogHasNext: Boolean = false,
    val loadingSections: Set<String> = emptySet(), val selected: String? = null, val blog: PhotoBlog? = null,
    val frame: Boolean = false, val blogLoading: Boolean = false, val blogError: UiText? = null)

// Connection-scoped state: changing a folder, search or account cancels its
// requests. Additional section pages never overwrite another section's data.
class PhotosController(parent: CoroutineScope, val service: PhotosService, val session: PhotoSession,
    application: android.content.Context? = null) {
    private val scope = CoroutineScope(parent.coroutineContext + SupervisorJob(parent.coroutineContext[Job]))
    private val mutable = MutableStateFlow(PhotosState())
    val state = mutable.asStateFlow()
    val downloads = application?.let {PhotoDownloads(it,scope,service)}
    val playback = application?.let {PlaybackPreferences(it)}
    private val application=application?.applicationContext
    private val preloader=this.application?.let {WifiOriginalPreloader(it,scope)}
    private var frameReturn: Pair<PhotosState,Map<String,Int>>? = null
    private var generation = 0L
    private var request: Job? = null
    private var detail: Job? = null
    private var detailGeneration = 0L
    private val pages = mutableMapOf("media" to 1,"folders" to 1,"blogs" to 1)
    private val additional = mutableMapOf<String,Job>()
    init { open(PhotoQuery(recursive=true)) }
    fun open(query: PhotoQuery, tab: Int = state.value.tab, frame: Boolean = false) {
        generation++
        request?.cancel(); detail?.cancel(); additional.values.forEach { it.cancel() }; additional.clear()
        pages.keys.forEach { pages[it]=1 }
        mutable.value = PhotosState(query=query,tab=tab,frame=frame,loading=true)
        val expected = generation
        request = scope.launch {
            try {
                val page = service.browse(query)
                if(generation == expected) mutable.value = PhotosState(query=query,tab=tab,frame=frame,selected=if(frame) page.media.firstOrNull()?.path else null,media=page.media,folders=page.folders,
                    blogs=page.blogs,total=page.total,hasNext=page.hasNext,folderHasNext=page.folderHasNext,blogHasNext=page.blogHasNext)
            } catch(e: CancellationException) { throw e }
            catch(e: Exception) { if(generation==expected) mutable.update { it.copy(loading=false,error=failureText(e)) } }
        }
    }
    fun more(section: String) {
        val current = state.value
        if(current.loading || section in current.loadingSections || when(section) {
                "media" -> !current.hasNext; "folders" -> !current.folderHasNext; "blogs" -> !current.blogHasNext; else -> true }) return
        val expected = generation
        val pageNumber = pages.getValue(section)+1
        mutable.update { it.copy(loadingSections=it.loadingSections+section,error=null) }
        additional[section] = scope.launch {
            try {
                val page = service.browse(current.query,pageNumber,section)
                if(generation!=expected) return@launch
                pages[section] = pageNumber
                mutable.update {
                    when(section) {
                        "media" -> it.copy(media=(it.media+page.media).distinctBy(Photo::path),hasNext=page.hasNext,total=page.total)
                        "folders" -> it.copy(folders=(it.folders+page.folders).distinctBy(PhotoFolder::path),folderHasNext=page.folderHasNext)
                        else -> it.copy(blogs=(it.blogs+page.blogs).distinctBy(PhotoBlog::path),blogHasNext=page.blogHasNext)
                    }.copy(loadingSections=it.loadingSections-section)
                }
            } catch(e: CancellationException) { throw e }
            catch(e: Exception) { if(generation==expected) mutable.update { it.copy(loadingSections=it.loadingSections-section,error=failureText(e)) } }
        }
    }
    fun select(path: String?) { mutable.update { it.copy(selected=path) } }
    suspend fun prefetch(photo: Photo?, images: coil.ImageLoader) {
        val context=application ?: return
        if(photo==null || photo.type!="image") return
        val request=photoPreviewRequest(context,photo,service,session)
        try {preloader?.preload(listOf(service.thumbnail(photo,session.largePreviewSize))) {images.execute(request)}}
        catch(e: CancellationException) {throw e}
        catch(_: Exception) { /* Optional prefetch never interrupts viewing. */ }
    }
    fun startFrame() {
        if(state.value.loading || state.value.frame) return
        frameReturn=state.value.copy(loadingSections=emptySet(),selected=null) to pages.toMap()
        open(state.value.query.copy(recursive=true,type="image"),frame=true)
    }
    fun closeViewer() {
        val saved=frameReturn
        if(state.value.frame && saved!=null) {
            generation++;request?.cancel();detail?.cancel();additional.values.forEach {it.cancel()};additional.clear()
            pages.clear();pages.putAll(saved.second)
            mutable.value=saved.first
            frameReturn=null
        } else select(null)
    }
    fun closeBlog() { detailGeneration++;detail?.cancel(); mutable.update { it.copy(blog=null,blogLoading=false,blogError=null) } }
    fun openBlog(post: PhotoBlog) {
        detailGeneration++
        detail?.cancel()
        mutable.update { it.copy(blog=post,blogLoading=true,blogError=null) }
        val expected = generation
        val expectedDetail = detailGeneration
        detail = scope.launch {
            try {
                val result = service.blog(post.path)
                if(generation==expected && detailGeneration==expectedDetail) mutable.update { it.copy(blog=result,blogLoading=false) }
            } catch(e: CancellationException) { throw e }
            catch(e: Exception) { if(generation==expected && detailGeneration==expectedDetail) mutable.update { it.copy(blogLoading=false,blogError=failureText(e)) } }
        }
    }
    fun close() { generation++;scope.cancel();frameReturn=null;mutable.value=PhotosState() }
}
