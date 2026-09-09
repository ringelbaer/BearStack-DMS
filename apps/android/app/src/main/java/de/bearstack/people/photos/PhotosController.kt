package de.bearstack.people.photos

import de.bearstack.people.text.*
import de.bearstack.people.data.remote.*
import de.bearstack.people.people.WifiOriginalPreloader
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*

data class PhotosState(val query: PhotoQuery = PhotoQuery(recursive=true), val tab: Int = 0, val loading: Boolean = false,
    val error: UiText? = null, val mediaPages: PhotoPages<Photo> = PhotoPages.media(),
    val folderPages: PhotoPages<PhotoFolder> = PhotoPages.folders(), val blogPages: PhotoPages<PhotoBlog> = PhotoPages.blogs(),
    val total: Int = 0, val loadingSections: Set<String> = emptySet(),
    val pageErrors: Map<String,PhotoPageFailure> = emptyMap(), val selected: String? = null, val blog: PhotoBlog? = null,
    val frame: Boolean = false, val blogLoading: Boolean = false, val blogError: UiText? = null,
    val scrollToKey: String? = null) {
    val media get() = mediaPages.items
    val folders get() = folderPages.items
    val blogs get() = blogPages.items
    val hasNext get() = mediaPages.hasNext
    val folderHasNext get() = folderPages.hasNext
    val blogHasNext get() = blogPages.hasNext
    fun section(name: String): PhotoPages<*> = when(name) {
        "media" -> mediaPages; "folders" -> folderPages; "blogs" -> blogPages
        else -> throw IllegalArgumentException("Unknown gallery section")
    }
}

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
    private var frameReturn: Pair<PhotosState,GalleryPosition?>? = null
    var gridPosition: GalleryPosition? = null
    private var visibleKeys: Set<String> = emptySet()
    fun galleryVisible(keys: Set<String>) { visibleKeys=keys }
    private fun protectedKeys(state: PhotosState,section: String): Set<String> {
        if(state.selected!=null) return if(section=="media") setOf(state.selected) else emptySet()
        val prefix=when(section) {"media" -> "photo:"; "folders" -> "folder:"; else -> "blog:"}
        return visibleKeys.filter {it.startsWith(prefix)}.mapTo(HashSet()) {it.removePrefix(prefix)}
    }
    private var generation = 0L
    private var request: Job? = null
    private var detail: Job? = null
    private var detailGeneration = 0L
    private val additional = mutableMapOf<String,Job>()
    init { open(PhotoQuery(recursive=true)) }
    fun open(query: PhotoQuery, tab: Int = state.value.tab, frame: Boolean = false) {
        generation++
        request?.cancel(); detail?.cancel(); additional.values.forEach { it.cancel() }; additional.clear()
        gridPosition=null;visibleKeys=emptySet()
        mutable.value = PhotosState(query=query,tab=tab,frame=frame,loading=true)
        val expected = generation
        request = scope.launch {
            try {
                val page = service.browse(query)
                validate(page,1)
                if(generation == expected) mutable.value = PhotosState(query=query,tab=tab,frame=frame,
                    selected=if(frame) page.media.firstOrNull()?.path else null,
                    mediaPages=PhotoPages.media().add(1,page.media,page.hasNext),
                    folderPages=PhotoPages.folders().add(1,page.folders,page.folderHasNext),
                    blogPages=PhotoPages.blogs().add(1,page.blogs,page.blogHasNext),total=page.total)
            } catch(e: CancellationException) { throw e }
            catch(e: Exception) { if(generation==expected) mutable.update { it.copy(loading=false,error=failureText(e)) } }
        }
    }
    private fun validate(page: PhotoPage, number: Int) {
        requireMessage(page.page==number && page.media.size<=96 && page.folders.size<=24 && page.blogs.size<=20 &&
            page.folders.all {it.previews.size<=2},de.bearstack.people.R.string.error_response_invalid)
    }
    fun more(section: String) { loadSection(section,false) }
    fun previous(section: String) { loadSection(section,true) }
    fun retryPage(section: String) { state.value.pageErrors[section]?.let {loadSection(section,it.previous)} }
    private fun loadSection(section: String, previous: Boolean, reset: Boolean = false): Job? {
        val current = state.value
        if(current.loading) return null
        if(section in current.loadingSections) return additional[section]
        val window=current.section(section)
        if(!reset && if(previous) !window.hasPrevious else !window.hasNext) return null
        val expected = generation
        val pageNumber = if(reset) 1 else if(previous) window.firstPage-1 else window.lastPage+1
        mutable.update { it.copy(loadingSections=it.loadingSections+section,pageErrors=it.pageErrors-section) }
        val job = scope.launch(start=CoroutineStart.LAZY) {
            try {
                val page = service.browse(current.query,pageNumber,section)
                validate(page,pageNumber)
                if(generation!=expected) return@launch
                mutable.update {
                    when(section) {
                        "media" -> it.copy(mediaPages=it.mediaPages.add(pageNumber,page.media,page.hasNext,reset,protectedKeys(it,section)),total=page.total)
                        "folders" -> it.copy(folderPages=it.folderPages.add(pageNumber,page.folders,page.folderHasNext,reset,protectedKeys(it,section)))
                        else -> it.copy(blogPages=it.blogPages.add(pageNumber,page.blogs,page.blogHasNext,reset,protectedKeys(it,section)))
                    }.copy(loadingSections=it.loadingSections-section)
                }
            } catch(e: CancellationException) { throw e }
            catch(e: Exception) { if(generation==expected) mutable.update {
                it.copy(loadingSections=it.loadingSections-section,pageErrors=it.pageErrors+(section to PhotoPageFailure(previous,failureText(e))))
            } }
        }
        additional[section]=job
        job.start()
        return job
    }
    // Resolve neighbours by identity after a possible eviction, never by a stale
    // list index. The viewer uses this for buttons and slideshow continuation.
    suspend fun neighbour(path: String, direction: Int, repeat: Boolean = false): Photo? {
        val expected=generation
        fun adjacent(): Photo? {
            val items=state.value.media
            val index=items.indexOfFirst {it.path==path}
            return if(index<0) null else items.getOrNull(index+direction)
        }
        adjacent()?.let {return it}
        loadSection("media",direction<0)?.join()
        if(generation!=expected) return null
        adjacent()?.let {return it}
        if(repeat && direction>0 && !state.value.hasNext && "media" !in state.value.pageErrors) {
            if(state.value.mediaPages.hasPrevious) loadSection("media",false,reset=true)?.join()
            if(generation==expected && "media" !in state.value.pageErrors) return state.value.media.firstOrNull()
        }
        return null
    }
    fun viewerAt(path: String) {
        if(state.value.selected!=null && state.value.media.any {it.path==path}) mutable.update {it.copy(selected=path)}
    }
    fun scrollConsumed() { mutable.update {it.copy(scrollToKey=null)} }
    fun select(path: String?) {
        mutable.update {if(path==null || it.media.any {photo -> photo.path==path}) it.copy(selected=path) else it}
    }
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
        frameReturn=state.value.copy(loadingSections=emptySet(),selected=null) to gridPosition
        open(state.value.query.copy(recursive=true),frame=true)
    }
    fun closeViewer() {
        val saved=frameReturn
        if(state.value.frame && saved!=null) {
            generation++;request?.cancel();detail?.cancel();additional.values.forEach {it.cancel()};additional.clear()
            gridPosition=saved.second
            mutable.value=saved.first
            frameReturn=null
        } else mutable.update {it.copy(scrollToKey=it.selected?.let {path -> "photo:$path"},selected=null)}
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
    fun close() {
        generation++;scope.cancel();additional.clear();request=null;detail=null
        frameReturn=null;gridPosition=null;visibleKeys=emptySet();mutable.value=PhotosState()
    }
}
