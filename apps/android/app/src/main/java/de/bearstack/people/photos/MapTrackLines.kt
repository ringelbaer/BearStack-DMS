package de.bearstack.people.photos

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.PointMode
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.unit.dp
import de.bearstack.people.data.remote.PhotoTrackGeometry
import kotlin.math.pow

internal fun trackColor(path: String): Color {
    if(path==photoRoutePath) return Color(0xff1976d2)
    val palette=listOf(0xff00897b,0xffe65100,0xff3949ab,0xffd81b60,0xff6d4c41,0xff7b1fa2)
    return Color(palette[(path.hashCode() and Int.MAX_VALUE)%palette.size])
}

@Composable
internal fun MapTrackLines(tracks: List<PhotoTrackGeometry>,camera: MapCamera,tilePixels: Double) {
    val lines=remember(tracks) {tracks.map {track ->Triple(trackColor(track.path),track.path==photoRoutePath,track.segments.map {segment ->
        segment.map {mapProject(it.latitude,it.longitude)}
    })}}
    Canvas(Modifier.fillMaxSize()) {
        val world=tilePixels*2.0.pow(camera.zoom)
        lines.forEach {(color,photoRoute,segments) ->
            val path=Path()
            val dots=mutableListOf<Offset>()
            segments.forEach segmentLoop@ {segment ->
                if(segment.isEmpty()) return@segmentLoop
                val origin=segment.first().x
                // Subpaths preserve every gap; world copies stay clipped by the map.
                for(shift in -1..1) {
                    var previous=origin
                    var x=wrapMapX(origin-camera.x+.5)-.5+camera.x+shift
                    segment.forEachIndexed {index,point ->
                        if(index>0) x+=wrapMapX(point.x-previous+.5)-.5
                        previous=point.x
                        val px=(size.width/2+(x-camera.x)*world).toFloat()
                        val py=(size.height/2+(point.y-camera.y)*world).toFloat()
                        if(segment.size==1) dots+=Offset(px,py)
                        else if(index==0) path.moveTo(px,py) else path.lineTo(px,py)
                    }
                }
            }
            val effect=if(photoRoute) PathEffect.dashPathEffect(floatArrayOf(8.dp.toPx(),6.dp.toPx())) else null
            drawPath(path,Color.White.copy(alpha=.9f),style=Stroke(6.dp.toPx(),cap=StrokeCap.Round,join=StrokeJoin.Round,pathEffect=effect))
            drawPath(path,color,style=Stroke(3.dp.toPx(),cap=StrokeCap.Round,join=StrokeJoin.Round,pathEffect=effect))
            if(dots.isNotEmpty()) {
                drawPoints(dots,PointMode.Points,Color.White,strokeWidth=8.dp.toPx(),cap=StrokeCap.Round)
                drawPoints(dots,PointMode.Points,color,strokeWidth=5.dp.toPx(),cap=StrokeCap.Round)
            }
        }
    }
}
