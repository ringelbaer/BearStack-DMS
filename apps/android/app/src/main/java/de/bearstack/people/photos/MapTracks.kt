package de.bearstack.people.photos

import de.bearstack.people.R
import de.bearstack.people.data.remote.*
import de.bearstack.people.text.UiText
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit

internal data class MapFocus(val revision: Long,val bounds: PhotoMapBounds)
internal data class MapTrackState(
    val pages: List<PhotoTrackPage> = emptyList(),val listLoading: Boolean = false,val listError: UiText? = null,
    val selected: Map<String,PhotoTrackFile> = emptyMap(),val geometry: Map<String,PhotoTrackGeometry> = emptyMap(),
    val loading: Set<String> = emptySet(),val errors: Map<String,UiText> = emptyMap(),val focus: MapFocus? = null,
) {
    val files get()=pages.flatMap {it.tracks}.distinctBy {it.path}
    val ready get()=pages.all {it.ready}
}

// A map owns at most 96 inventory entries, 256 selected names and 8192 displayed
// coordinates. Cancelling the map or changing the viewport cancels pending work.
internal class MapTracks(parent: CoroutineScope,private val service: PhotosService,private val path: String) {
    private val scope=CoroutineScope(parent.coroutineContext+SupervisorJob(parent.coroutineContext[Job]))
    private val mutable=MutableStateFlow(MapTrackState())
    val state=mutable.asStateFlow()
    private var inventory: Job?=null
    private var inventoryGeneration=0L
    private var geometry: Job?=null
    private val geometryPermits=Semaphore(2)
    private var viewport: PhotoMapBounds?=null
    private var generation=0L
    private var focusRevision=0L
    private var pendingFocus: String?=null
    private var visible=emptySet<String>()
    private var lastListRequest=false to false
    fun visible(paths: Set<String>) {visible=paths}
    fun open() {if(state.value.pages.isEmpty()) load(false)}
    fun retryList() {
        if(state.value.listError!=null) load(lastListRequest.first,lastListRequest.second) else load(false,reset=true)
    }
    fun hideList() {inventoryGeneration++;inventory?.cancel();mutable.update {it.copy(listLoading=false)}}
    fun load(before: Boolean,reset: Boolean=false) {
        val current=state.value
        if(current.listLoading) return
        val edge=if(before) current.pages.firstOrNull() else current.pages.lastOrNull()
        if(!reset && edge!=null && !(if(before) edge.hasPrevious else edge.hasNext)) return
        if(!reset && current.pages.size>=3) {
            val evicted=if(before) current.pages.last() else current.pages.first()
            if(evicted.tracks.any {it.path in visible}) return
        }
        val cursor=if(reset) "" else if(before) edge?.previousCursor.orEmpty() else edge?.cursor.orEmpty()
        val expectedInventory=++inventoryGeneration
        lastListRequest=before to reset
        mutable.update {it.copy(listLoading=true,listError=null)}
        inventory=scope.launch {
            try {
                var nextCursor=cursor
                var page: PhotoTrackPage
                while(true) {
                    page=service.tracks(path,nextCursor,before)
                    require(page.tracks.size<=32)
                    val more=if(before) page.hasPrevious else page.hasNext
                    val next=if(before) page.previousCursor else page.cursor
                    require(!more || next.isNotBlank() && next!=nextCursor)
                    if(page.tracks.isNotEmpty() || !more) break
                    // Newly private tracks may occupy a whole candidate batch.
                    // Continue without evicting visible names for an empty batch.
                    nextCursor=next
                    currentCoroutineContext().ensureActive()
                }
                if(inventoryGeneration!=expectedInventory) return@launch
                mutable.update {
                    val evicted=if(before) it.pages.lastOrNull() else it.pages.firstOrNull()
                    if(!reset && it.pages.size>=3 && evicted?.tracks?.any {file -> file.path in visible}==true)
                        it.copy(listLoading=false)
                    else it.copy(pages=if(reset) listOf(page) else if(before) (listOf(page)+it.pages).take(3)
                        else (it.pages+page).takeLast(3),listLoading=false)
                }
            } catch(e: CancellationException) {throw e}
            catch(_: Exception) {if(inventoryGeneration==expectedInventory) mutable.update {it.copy(listLoading=false,listError=UiText(R.string.photos_tracks_list_error))}}
        }
    }
    fun toggle(file: PhotoTrackFile) {
        val selected=state.value.selected.toMutableMap()
        if(selected.remove(file.path)==null) {
            if(selected.size>=256) return
            selected[file.path]=file;pendingFocus=file.path
        } else if(pendingFocus==file.path) pendingFocus=null
        mutable.update {it.copy(selected=selected,geometry=emptyMap(),errors=emptyMap())}
        refresh()
    }
    fun clear() {
        pendingFocus=null
        mutable.update {it.copy(selected=emptyMap(),geometry=emptyMap(),errors=emptyMap())}
        refresh()
    }
    fun viewport(bounds: PhotoMapBounds) {
        if(viewport==bounds) return
        viewport=bounds;refresh()
    }
    fun refresh() {
        geometry?.cancel()
        val expected=++generation
        val files=state.value.selected.values.toList()
        mutable.update {it.copy(loading=files.mapTo(HashSet()) {f -> f.path},errors=emptyMap())}
        if(files.isEmpty()) return
        val extent=viewport
        val budget=(8192/files.size).coerceAtLeast(32)
        geometry=scope.launch {
            files.forEach {file ->launch {
                geometryPermits.withPermit {
                    try {
                        val result=service.track(file.path,extent,budget)
                        require(result.path==file.path && result.segments.sumOf {it.size}<=budget)
                        if(generation==expected) mutable.update {
                            val focus=if(pendingFocus==file.path && result.bounds!=null) {
                                pendingFocus=null;MapFocus(++focusRevision,result.bounds)
                            } else it.focus
                            it.copy(geometry=it.geometry+(file.path to result),loading=it.loading-file.path,focus=focus)
                        }
                    } catch(e: CancellationException) {throw e}
                    catch(e: Exception) {
                        if(generation==expected) mutable.update {
                            val message=if(e is ApiFailure && e.code=="gpx_too_large") R.string.photos_track_too_large else R.string.photos_track_error
                            it.copy(geometry=it.geometry-file.path,loading=it.loading-file.path,errors=it.errors+(file.path to UiText(message)))
                        }
                    }
                }
            }}
        }
    }
    fun close() {generation++;inventoryGeneration++;scope.cancel();mutable.value=MapTrackState();visible=emptySet()}
}
