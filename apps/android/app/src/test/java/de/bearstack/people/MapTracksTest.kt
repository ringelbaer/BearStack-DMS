package de.bearstack.people

import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.*
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import org.junit.Assert.*
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MapTracksTest {
    private fun file(id: Int)=PhotoTrackFile("$id.gpx","Track $id","2026-09-09T00:00:00Z",100)
    private open inner class Source: PhotosService {
        override suspend fun session()=throw UnsupportedOperationException()
        override suspend fun browse(query: PhotoQuery,page: Int,section: String)=throw UnsupportedOperationException()
        override suspend fun info(path: String)=throw UnsupportedOperationException()
        override suspend fun blog(path: String)=throw UnsupportedOperationException()
        override fun thumbnail(photo: Photo,size: Int)=""
        override fun original(photo: Photo)=""
        override suspend fun tracks(path: String,cursor: String,before: Boolean): PhotoTrackPage {
            val id=cursor.toIntOrNull() ?: 0
            val start=if(before) (id-32).coerceAtLeast(1) else id+1
            val end=if(before) id-1 else (start+31).coerceAtMost(10000)
            val files=(start..end).map(::file)
            return PhotoTrackPage(files,"$end","$start",end<10000,start>1,true)
        }
        override suspend fun track(path: String,bounds: PhotoMapBounds?,points: Int)=
            PhotoTrackGeometry(path,path,PhotoMapBounds(1.0,2.0,3.0,4.0),listOf(List(points) {PhotoMapPoint(2.0,3.0)}),10000,true,0)
    }
    @Test fun trackInventoryScrollsBothWaysWithoutGrowingOrEvictingVisibleNames()=runTest {
        val tracks=MapTracks(this,Source(),"")
        tracks.open();runCurrent()
        repeat(100) {
            tracks.visible(setOf(tracks.state.value.files.last().path))
            tracks.load(false);runCurrent()
            assertTrue(tracks.state.value.files.size<=96)
        }
        val last=tracks.state.value.files.last().path
        tracks.visible(setOf(tracks.state.value.files.first().path))
        tracks.load(false);runCurrent()
        assertEquals(last,tracks.state.value.files.last().path)
        repeat(100) {
            tracks.visible(setOf(tracks.state.value.files.first().path))
            tracks.load(true);runCurrent()
            assertTrue(tracks.state.value.files.size<=96)
        }
        assertEquals("1.gpx",tracks.state.value.files.first().path)
        tracks.close()
    }
    @Test fun emptyPrivateBatchesAdvanceAndClosingTheSheetCancelsInventory()=runTest {
        val gate=CompletableDeferred<Unit>()
        var calls=0
        val source=object:Source() {
            override suspend fun tracks(path: String,cursor: String,before: Boolean): PhotoTrackPage {
                calls++
                if(calls<3) return PhotoTrackPage(emptyList(),"${calls*32}","${(calls-1)*32+1}",true,calls>1,true)
                gate.await()
                return super.tracks(path,cursor,before)
            }
        }
        val tracks=MapTracks(this,source,"")
        tracks.open();runCurrent()
        assertEquals(3,calls);assertTrue(tracks.state.value.listLoading)
        tracks.hideList();runCurrent();gate.complete(Unit);runCurrent()
        assertFalse(tracks.state.value.listLoading);assertTrue(tracks.state.value.pages.isEmpty())
        tracks.open();runCurrent()
        assertEquals(32,tracks.state.value.files.size)
        tracks.close()
    }
    @Test fun geometryHasTwoRequestsAndOneCoordinateBudgetAcrossAllTracks()=runTest {
        val gate=CompletableDeferred<Unit>()
        var active=0;var peak=0
        val source=object:Source() {
            override suspend fun track(path: String,bounds: PhotoMapBounds?,points: Int): PhotoTrackGeometry {
                active++;peak=maxOf(peak,active)
                try {gate.await();return super.track(path,bounds,points)} finally {active--}
            }
        }
        val tracks=MapTracks(this,source,"")
        repeat(5) {tracks.toggle(file(it+1))};runCurrent()
        assertEquals(2,peak);assertEquals(2,active)
        gate.complete(Unit);runCurrent()
        assertEquals(5,tracks.state.value.geometry.size)
        assertTrue(tracks.state.value.geometry.values.sumOf {it.segments.sumOf {line -> line.size}}<=8192)
        assertTrue(tracks.state.value.loading.isEmpty())
        tracks.clear();runCurrent()
        assertTrue(tracks.state.value.geometry.isEmpty());assertTrue(tracks.state.value.selected.isEmpty())
        tracks.close()
    }
    @Test fun staleViewportResponsesAndRemovedTracksCannotReturn()=runTest {
        val old=CompletableDeferred<Unit>()
        val bounds=PhotoMapBounds(10.0,20.0,30.0,40.0)
        val source=object:Source() {
            override suspend fun track(path: String,bounds: PhotoMapBounds?,points: Int): PhotoTrackGeometry {
                if(bounds==null) withContext(NonCancellable) {old.await()}
                return super.track(path,bounds,points).copy(name=if(bounds==null) "old" else "current")
            }
        }
        val tracks=MapTracks(this,source,"")
        tracks.toggle(file(1));runCurrent()
        tracks.viewport(bounds);runCurrent()
        assertEquals("current",tracks.state.value.geometry.getValue("1.gpx").name)
        old.complete(Unit);runCurrent()
        assertEquals("current",tracks.state.value.geometry.getValue("1.gpx").name)
        tracks.toggle(file(1));runCurrent()
        assertTrue(tracks.state.value.geometry.isEmpty())
        tracks.close()
    }
    @Test fun fittingSeveralTracksPreservesWideAndDatelineExtents() {
        val dateline=unionMapBounds(listOf(PhotoMapBounds(-2.0,170.0,1.0,-175.0),PhotoMapBounds(0.0,178.0,3.0,-165.0)))!!
        assertEquals(-2.0,dateline.south,1e-9);assertEquals(3.0,dateline.north,1e-9)
        assertEquals(170.0,dateline.west,1e-9);assertEquals(-165.0,dateline.east,1e-9)
        val wide=unionMapBounds(listOf(PhotoMapBounds(0.0,-170.0,1.0,170.0),PhotoMapBounds(2.0,169.0,4.0,175.0)))!!
        assertEquals(0.0,wide.south,1e-9);assertEquals(4.0,wide.north,1e-9)
        assertEquals(-170.0,wide.west,1e-9);assertEquals(175.0,wide.east,1e-9)
    }
}
