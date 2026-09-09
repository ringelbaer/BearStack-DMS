package de.bearstack.people.photos

import de.bearstack.people.data.remote.PhotoMapBounds
import kotlin.math.*

internal const val MAP_MAX_LATITUDE = 85.05112878
internal data class MapCoordinate(val x: Double, val y: Double)
internal data class MapCamera(val x: Double = .5, val y: Double = .5, val zoom: Double = 2.0) {
    fun move(dx: Double, dy: Double, scale: Double, tilePixels: Double): MapCamera {
        val world=tilePixels * 2.0.pow(zoom)
        return copy(x=wrapMapX(x-dx/world),y=(y-dy/world).coerceIn(0.0,1.0),
            zoom=(zoom+log2(scale.coerceIn(.1,10.0))).coerceIn(2.0,18.0))
    }
    fun bounds(width: Double, height: Double, tilePixels: Double): PhotoMapBounds {
        val world=tilePixels * 2.0.pow(zoom)
        val halfX=width/2/world
        val south=mapLatitude((y+height/2/world).coerceAtMost(1.0))
        val north=mapLatitude((y-height/2/world).coerceAtLeast(0.0))
        return PhotoMapBounds(south,if(halfX>=.5)-180.0 else wrapMapX(x-halfX)*360-180,
            north,if(halfX>=.5)180.0 else wrapMapX(x+halfX)*360-180)
    }
}
internal fun wrapMapX(x: Double) = ((x % 1)+1)%1
internal fun expandMapBounds(bounds: PhotoMapBounds): PhotoMapBounds {
    val west=bounds.west-1e-9
    val east=bounds.east+1e-9
    val world=bounds.east-bounds.west>=359.999999
    return PhotoMapBounds((bounds.south-1e-9).coerceAtLeast(-90.0),
        if(world)-180.0 else if(west< -180)west+360 else west,
        (bounds.north+1e-9).coerceAtMost(90.0),
        if(world)180.0 else if(east>180)east-360 else east)
}
internal fun mapProject(latitude: Double, longitude: Double): MapCoordinate {
    val sine=sin(Math.toRadians(latitude.coerceIn(-MAP_MAX_LATITUDE,MAP_MAX_LATITUDE)))
    return MapCoordinate((longitude+180)/360,(.5-ln((1+sine)/(1-sine))/(4*PI)).coerceIn(0.0,1.0))
}
internal fun mapLatitude(y: Double) = Math.toDegrees(atan(sinh(PI*(1-2*y))))
internal fun fitMap(bounds: PhotoMapBounds, width: Double, height: Double, tilePixels: Double): MapCamera {
    val nw=mapProject(bounds.north,bounds.west)
    val se=mapProject(bounds.south,bounds.east)
    val east=if(bounds.west>bounds.east) se.x+1 else se.x
    val zoom=min(log2((width*.75).coerceAtLeast(1.0)/tilePixels/(east-nw.x).coerceAtLeast(1e-8)),
        log2((height*.75).coerceAtLeast(1.0)/tilePixels/(se.y-nw.y).coerceAtLeast(1e-8))).coerceIn(2.0,16.0)
    return MapCamera(wrapMapX((nw.x+east)/2),(nw.y+se.y)/2,zoom)
}
internal data class MapTile(val zoom: Int,val x: Int,val y: Int,val left: Double,val top: Double,val size: Double,val column: Int) {
    val url get()="https://tile.openstreetmap.org/$zoom/$x/$y.png"
}
// Enumerates only tiles intersecting the viewport. No neighbour prefetch.
internal fun visibleMapTiles(camera: MapCamera,width: Double,height: Double,tilePixels: Double): List<MapTile> {
    val zoom=floor(camera.zoom).toInt()
    val count=1 shl zoom
    val size=tilePixels*2.0.pow(camera.zoom-zoom)
    val left=camera.x*count*size-width/2
    val top=camera.y*count*size-height/2
    return buildList {
        for(y in floor(top/size).toInt()..floor((top+height-1)/size).toInt()) {
            if(y !in 0 until count) continue
            for(x in floor(left/size).toInt()..floor((left+width-1)/size).toInt()) {
                add(MapTile(zoom,((x%count)+count)%count,y,x*size-left,y*size-top,size,x))
            }
        }
    }
}

// Union geographic intervals by excluding the largest uncovered longitude gap.
// Using only corner points would incorrectly shrink a track spanning >180 degrees.
internal fun unionMapBounds(bounds: List<PhotoMapBounds>): PhotoMapBounds? {
    if(bounds.isEmpty()) return null
    val south=bounds.minOf {it.south};val north=bounds.maxOf {it.north}
    if(bounds.any {it.east-it.west>=359.999999}) return PhotoMapBounds(south,-180.0,north,180.0)
    val intervals=bounds.flatMap {
        if(it.west<=it.east) listOf((it.west+180) to (it.east+180))
        else listOf((it.west+180) to 360.0,0.0 to (it.east+180))
    }.sortedBy {it.first}
    val merged=mutableListOf<Pair<Double,Double>>()
    for(interval in intervals) {
        val last=merged.lastOrNull()
        if(last!=null && interval.first<=last.second) merged[merged.lastIndex]=last.first to max(last.second,interval.second)
        else merged+=interval
    }
    var gap=-1.0;var west=-180.0;var east=180.0
    for(i in merged.indices) {
        val next=merged[(i+1)%merged.size].first+(if(i==merged.lastIndex)360 else 0)
        val width=next-merged[i].second
        if(width>gap) {gap=width;west=wrapMapX(next/360)*360-180;east=merged[i].second-180}
    }
    return if(gap<=1e-9) PhotoMapBounds(south,-180.0,north,180.0) else PhotoMapBounds(south,west,north,east)
}
