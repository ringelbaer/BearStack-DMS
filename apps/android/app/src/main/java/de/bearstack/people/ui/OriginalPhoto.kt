package de.bearstack.people.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.gestures.detectVerticalDragGestures
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.layout.*
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.*
import androidx.compose.ui.unit.dp
import coil.ImageLoader
import coil.compose.AsyncImage
import coil.request.ImageRequest
import coil.size.Scale
import de.bearstack.people.data.remote.FaceBounds

@Composable
internal fun OriginalPhoto(model: Any?, images: ImageLoader, bounds: FaceBounds?, zoom: Float,
    onZoom: (Float) -> Unit, onDrag: (Float) -> Unit, modifier: Modifier = Modifier,
    onNewTouch: (() -> Unit)? = null) {
    var imageSize by remember(model) { mutableStateOf(Size.Zero) }
    var viewport by remember { mutableStateOf(Size.Zero) }
    var loading by remember(model) { mutableStateOf(true) }
    var failed by remember(model) { mutableStateOf(false) }
    val context=LocalContext.current
    // Decode once at a bounded resolution, never fetch or decode again for each drag update.
    val request=remember(context,model) { ImageRequest.Builder(context).data(model).size(2048).scale(Scale.FIT).build() }
    val transform=originalTransform(viewport,imageSize,bounds,zoom)
    val drag by rememberUpdatedState(onDrag)
    val newTouch by rememberUpdatedState(onNewTouch)
    Box(modifier.clipToBounds().testTag("original-photo").onSizeChanged { viewport=Size(it.width.toFloat(),it.height.toFloat()) }
        .semantics {
            stateDescription="Vergrößerung ${(transform.scale*100).toInt()} Prozent"
            if(bounds!=null && !loading && !failed) customActions=listOf(
                CustomAccessibilityAction("Zum Gesicht vergrößern") { onZoom(1f);true },
                CustomAccessibilityAction("Ganzes Foto anzeigen") { onZoom(0f);true })
        }
        .pointerInput(model) {
            awaitEachGesture {
                val down=awaitFirstDown(requireUnconsumed=false,pass=PointerEventPass.Initial)
                // A held pointer belongs to the covered tile; a new down on the overlay
                // is a second finger, which must not start a competing preview gesture.
                newTouch?.let { down.consume();it() }
            }
        }
        .pointerInput(model) { detectVerticalDragGestures { change, dy -> change.consume();drag(dy) } },
        contentAlignment=Alignment.Center) {
        AsyncImage(request,"Originalfoto",imageLoader=images,
            modifier=Modifier.fillMaxSize().graphicsLayer {
                scaleX=transform.scale;scaleY=transform.scale
                translationX=transform.translation.x;translationY=transform.translation.y
            },contentScale=ContentScale.Fit,
            onLoading={loading=true;failed=false;imageSize=Size.Zero},
            onSuccess={
                loading=false;failed=false
                // Coil applies EXIF before reporting drawable dimensions, just like face detection.
                imageSize=Size(it.result.drawable.intrinsicWidth.toFloat(),it.result.drawable.intrinsicHeight.toFloat())
            },onError={loading=false;failed=true;imageSize=Size.Zero})
        if(!loading && !failed) transform.face?.let { face ->
            Canvas(Modifier.fillMaxSize().testTag("original-face-box")) {
                // Constant screen-space width: the outline stays fine even at maximum zoom.
                drawRect(Color.Black.copy(alpha=.7f),face.topLeft,face.size,style=Stroke(2.dp.toPx()))
                drawRect(Color.White,face.topLeft,face.size,style=Stroke(1.dp.toPx()))
            }
        }
        if(loading) CircularProgressIndicator()
        if(failed) Text("Originalfoto konnte nicht geladen werden. Vorschau schließen oder loslassen und erneut halten.",
            color=Color.White,modifier=Modifier.padding(24.dp))
    }
}
