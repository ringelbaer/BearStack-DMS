package de.bearstack.people

import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.*
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import org.junit.Assert.*
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MapPhotoRouteTest {
    private class Source:PhotosService {
        val calls=mutableListOf<Pair<PhotoQuery,PhotoMapBounds?>>()
        var handler:suspend (PhotoMapBounds?)->PhotoRouteData={extent ->PhotoRouteData(
            PhotoTrackGeometry("","",extent,listOf(listOf(PhotoMapPoint(1.0,2.0))),1,false,0),12,1000)}
        override suspend fun route(query:PhotoQuery,bounds:PhotoMapBounds?,points:Int):PhotoRouteData {
            assertEquals(4096,points);calls+=query to bounds
            return handler(bounds)
        }
        override suspend fun session()=throw UnsupportedOperationException()
        override suspend fun browse(query:PhotoQuery,page:Int,section:String)=throw UnsupportedOperationException()
        override suspend fun info(path:String)=throw UnsupportedOperationException()
        override suspend fun blog(path:String)=throw UnsupportedOperationException()
        override fun thumbnail(photo:Photo,size:Int)=""
        override fun original(photo:Photo)=""
    }
    @Test fun routeIsOptInAndKeepsFiltersAndViewportWithBoundedGeometry()=runTest {
        val source=Source();val query=PhotoQuery(path="trip",query="tag:sea",type="image")
        val route=MapPhotoRoute(this,source,query)
        val extent=PhotoMapBounds(0.0,0.0,3.0,3.0)
        route.viewport(extent);runCurrent();assertTrue(source.calls.isEmpty())
        route.enable(true);runCurrent()
        assertEquals(listOf(query to extent),source.calls)
        assertEquals(photoRoutePath,route.state.value.data!!.geometry.path)
        route.viewport(extent);route.enable(true);runCurrent();assertEquals(1,source.calls.size)
        source.handler={throw java.io.IOException("failure")}
        route.refresh();runCurrent();assertNotNull(route.state.value.error)
        assertEquals(12,route.state.value.data!!.totalMedia)
        route.enable(false);runCurrent();assertNull(route.state.value.data);assertNull(route.state.value.error)
        route.close()
    }
    @Test fun supersededAndHiddenRequestsCannotReturnOldGeometryOrExceedOneCall()=runTest {
        val source=Source();val entered=CompletableDeferred<Unit>();val release=CompletableDeferred<Unit>()
        val first=source.handler
        var active=0;var peak=0
        source.handler={extent ->
            active++;peak=maxOf(peak,active)
            try {
                if(source.calls.size==1) withContext(NonCancellable) {entered.complete(Unit);release.await()}
                first(extent)
            } finally {active--}
        }
        val route=MapPhotoRoute(this,source,PhotoQuery())
        route.enable(true);runCurrent();entered.await()
        route.viewport(PhotoMapBounds(0.0,1.0,2.0,3.0));runCurrent()
        assertEquals(1,source.calls.size)
        route.enable(false);release.complete(Unit);runCurrent()
        assertNull(route.state.value.data);assertFalse(route.state.value.loading)
        route.enable(true);runCurrent();assertNotNull(route.state.value.data)
        assertEquals(1,peak);assertEquals(2,source.calls.size)
        route.close();assertNull(route.state.value.data)
    }
}
