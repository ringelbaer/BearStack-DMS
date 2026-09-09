package de.bearstack.people

import de.bearstack.people.data.remote.PhotoMapBounds
import de.bearstack.people.photos.*
import org.junit.Assert.*
import org.junit.Test
import kotlin.math.pow

class MapGeometryTest {
    @Test fun coincidentLocationsAtTheDateLineRemainSelectable() {
        for(lon in listOf(-180.0,180.0)) {
            val area=expandMapBounds(PhotoMapBounds(2.0,lon,2.0,lon))
            assertTrue(area.west>179.999 && area.east< -179.999)
            assertTrue(area.south<2 && area.north>2)
        }
        val world=expandMapBounds(PhotoMapBounds(-90.0,-180.0,90.0,180.0))
        assertEquals(PhotoMapBounds(-90.0,-180.0,90.0,180.0),world)
    }
    @Test fun projectionRoundTripsAndFitsAcrossDateLine() {
        for(lat in listOf(-85.0,-52.5,0.0,52.5,85.0)) assertEquals(lat,mapLatitude(mapProject(lat,0.0).y),.000001)
        assertTrue(mapProject(90.0,0.0).y.isFinite())
        val camera=fitMap(PhotoMapBounds(-2.0,179.0,2.0,-179.0),1080.0,1600.0,768.0)
        assertTrue(camera.x<.01 || camera.x>.99)
        val visible=camera.bounds(1080.0,1600.0,768.0)
        assertTrue(visible.west>visible.east)
        assertTrue(visible.south< -2 && visible.north>2)
        assertTrue(camera.zoom>5)
    }
    @Test fun tileRequestsStayWithinViewportAndWrapAtDateLine() {
        for(zoom in listOf(2.0,7.4,18.0)) {
            val camera=MapCamera(.999,.2,zoom)
            val tiles=visibleMapTiles(camera,1080.0,1600.0,768.0)
            assertTrue(tiles.size in 1..12)
            for(tile in tiles) {
                assertTrue(tile.left<1080 && tile.left+tile.size>0)
                assertTrue(tile.top<1600 && tile.top+tile.size>0)
                assertTrue(tile.x>=0 && tile.x<2.0.pow(tile.zoom))
                assertTrue(tile.y>=0 && tile.y<2.0.pow(tile.zoom))
                assertTrue(tile.url.startsWith("https://tile.openstreetmap.org/"))
            }
        }
    }
    @Test fun movementClampsPolesZoomAndWrapsLongitude() {
        val camera=MapCamera(.001,.01,18.0).move(1e15,1e15,10.0,768.0)
        assertTrue(camera.x>=0 && camera.x<1)
        assertEquals(0.0,camera.y,0.0)
        assertEquals(18.0,camera.zoom,0.0)
        assertEquals(2.0,MapCamera(zoom=2.0).move(0.0,0.0,.1,768.0).zoom,0.0)
    }
}
