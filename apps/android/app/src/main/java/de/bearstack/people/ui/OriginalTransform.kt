package de.bearstack.people.ui

import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.Size
import de.bearstack.people.data.remote.FaceBounds
import kotlin.math.pow

internal data class OriginalTransform(val scale: Float, val translation: Offset, val face: Rect?)

/** Fit the oriented image, then zoom towards the selected face without panning beyond its edges. */
internal fun originalTransform(viewport: Size, image: Size, bounds: FaceBounds?, progress: Float): OriginalTransform {
    if(viewport.width<=0 || viewport.height<=0 || image.width<=0 || image.height<=0 || bounds==null)
        return OriginalTransform(1f,Offset.Zero,null)
    val fit=minOf(viewport.width/image.width,viewport.height/image.height)
    val width=image.width*fit; val height=image.height*fit
    val center=Offset(viewport.width/2,viewport.height/2)
    val face=Rect((viewport.width-width)/2+bounds.x*width, (viewport.height-height)/2+bounds.y*height,
        (viewport.width-width)/2+(bounds.x+bounds.width)*width, (viewport.height-height)/2+(bounds.y+bounds.height)*height)
    // Keep context around the face; cap magnification for very small detections.
    val maxScale=minOf(viewport.width/(face.width*1.5f),viewport.height/(face.height*1.5f)).coerceIn(1f,12f)
    val scale=maxScale.pow(progress.coerceIn(0f,1f))
    val fraction=if(maxScale>1f) (scale-1f)/(maxScale-1f) else 0f
    val target=(center-face.center)*maxScale*fraction
    val xLimit=((width*scale-viewport.width)/2).coerceAtLeast(0f)
    val yLimit=((height*scale-viewport.height)/2).coerceAtLeast(0f)
    val translation=Offset(target.x.coerceIn(-xLimit,xLimit),target.y.coerceIn(-yLimit,yLimit))
    return OriginalTransform(scale,translation,Rect(
        center+(face.topLeft-center)*scale+translation,
        center+(face.bottomRight-center)*scale+translation))
}
