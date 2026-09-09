package de.bearstack.people.photos

import de.bearstack.people.R
import de.bearstack.people.data.remote.*
import de.bearstack.people.text.UiText
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit

internal const val photoRoutePath="\u0000photo-route"
internal data class MapRouteState(val enabled: Boolean=false,val loading: Boolean=false,
    val data: PhotoRouteData?=null,val error: UiText?=null)

// An opt-in route has one request and at most 4096 coordinates. Changing the
// folder or hiding the layer cancels it; stale responses cannot restore a layer.
internal class MapPhotoRoute(parent: CoroutineScope,private val service: PhotosService,private val query: PhotoQuery) {
    private val scope=CoroutineScope(parent.coroutineContext+SupervisorJob(parent.coroutineContext[Job]))
    private val mutable=MutableStateFlow(MapRouteState())
    val state=mutable.asStateFlow()
    private var viewport:PhotoMapBounds?=null
    private var generation=0L
    private var request:Job?=null
    private val permit=Semaphore(1)
    fun enable(enabled:Boolean) {
        if(enabled==state.value.enabled) return
        generation++;request?.cancel()
        mutable.value=MapRouteState(enabled=enabled)
        if(enabled) refresh()
    }
    fun viewport(bounds:PhotoMapBounds) {
        if(bounds==viewport) return
        viewport=bounds
        if(state.value.enabled) refresh()
    }
    fun refresh() {
        if(!state.value.enabled) return
        val expected=++generation
        val bounds=viewport
        request?.cancel()
        mutable.update {it.copy(loading=true,error=null)}
        request=scope.launch {permit.withPermit {
            try {
                val data=service.route(query,bounds,4096)
                require(data.geometry.segments.sumOf {it.size}<=4096)
                if(generation==expected) mutable.update {it.copy(loading=false,data=data.copy(
                    geometry=data.geometry.copy(path=photoRoutePath)),error=null)}
            } catch(e:CancellationException) {throw e}
            catch(e:Exception) {
                if(generation==expected) mutable.update {it.copy(loading=false,error=UiText(when {
                    e is ApiFailure && e.code=="search_too_broad" -> R.string.error_search_broad
                    e is ApiFailure && e.code=="map_index_not_ready" -> R.string.photos_map_index_pending
                    else -> R.string.photos_route_error
                }))}
            }
        }}
    }
    fun close() {generation++;scope.cancel();mutable.value=MapRouteState()}
}
