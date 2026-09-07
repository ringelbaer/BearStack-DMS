package de.bearstack.people

import android.graphics.Bitmap
import androidx.compose.foundation.layout.*
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.toPixelMap
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.dp
import androidx.test.platform.app.InstrumentationRegistry
import android.media.ExifInterface
import coil.ImageLoader
import de.bearstack.people.data.remote.FaceBounds
import de.bearstack.people.data.remote.Person
import de.bearstack.people.ui.FaceGrid
import de.bearstack.people.ui.OriginalPhoto
import de.bearstack.people.ui.PersonSwipeArea
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.File

class OriginalPhotoTest {
    @get:Rule val compose=createComposeRule()

    @Test fun exifRotatedOriginalUsesOrientedDimensionsForOutline() {
        val context=InstrumentationRegistry.getInstrumentation().targetContext
        val loader=ImageLoader(context)
        val file=File.createTempFile("original-orientation-", ".jpg",context.cacheDir)
        val bitmap=Bitmap.createBitmap(600,300,Bitmap.Config.ARGB_8888).apply { eraseColor(android.graphics.Color.BLUE) }
        try {
            file.outputStream().use { bitmap.compress(Bitmap.CompressFormat.JPEG,90,it) }
            ExifInterface(file.absolutePath).apply {
                setAttribute(ExifInterface.TAG_ORIENTATION,ExifInterface.ORIENTATION_ROTATE_90.toString())
                saveAttributes()
            }
            compose.setContent { MaterialTheme {
                OriginalPhoto(file,loader,FaceBounds(.1f,.2f,.2f,.3f),0f,{},{},Modifier.size(300.dp,400.dp))
            } }
            compose.waitUntil(5000) { compose.onAllNodesWithTag("original-face-box").fetchSemanticsNodes().size==1 }
            val pixels=compose.onNodeWithTag("original-photo").captureToImage().toPixelMap()
            // After EXIF rotation the 1:2 photo fits at x=50..250, y=0..400 dp.
            // Its face box starts at (70,80) dp, so (90,80) lies on the top stroke.
            val x=(pixels.width*90/300f).toInt();val y=(pixels.height*.2f).toInt()
            assertTrue((-3..3).any { delta -> pixels[x,y+delta].let { it.red>.9f && it.green>.9f && it.blue>.9f } })
        } finally { loader.shutdown();file.delete();bitmap.recycle() }
    }

    @Test fun outlineMatchesFittedOriginalAndPreviewSupportsAccessibleAndDragZoom() {
        val context=InstrumentationRegistry.getInstrumentation().targetContext
        val loader=ImageLoader(context)
        val bitmap=Bitmap.createBitmap(600,300,Bitmap.Config.ARGB_8888).apply { eraseColor(android.graphics.Color.BLUE) }
        var zoom by mutableFloatStateOf(0f)
        try {
            compose.setContent { MaterialTheme {
                val distance=with(LocalDensity.current) { 240.dp.toPx() }
                OriginalPhoto(bitmap,loader,FaceBounds(.1f,.2f,.2f,.3f),zoom,{zoom=it},
                    {zoom=(zoom-it/distance).coerceIn(0f,1f)},Modifier.size(300.dp,400.dp))
            } }
            compose.waitUntil(5000) { compose.onAllNodesWithTag("original-face-box").fetchSemanticsNodes().size==1 }
            val preview=compose.onNodeWithTag("original-photo")
            val pixels=preview.captureToImage().toPixelMap()
            // A 2:1 original fits at y=125..275 dp. Box top is 125+.2*150=155 dp.
            val x=(pixels.width*.2f).toInt();val y=(pixels.height*155/400f).toInt()
            assertTrue((-3..3).any { delta -> pixels[x,y+delta].let { it.red>.9f && it.green>.9f && it.blue>.9f } })
            val actions=preview.fetchSemanticsNode().config[SemanticsActions.CustomActions]
            compose.runOnIdle { assertTrue(actions.single {it.label=="Zum Gesicht vergrößern"}.action()) }
            compose.runOnIdle { assertEquals(1f,zoom,0f) }
            preview.performTouchInput { swipeDown() }
            compose.runOnIdle { assertTrue(zoom<1f) }
            compose.runOnIdle { assertTrue(actions.single {it.label=="Ganzes Foto anzeigen"}.action()) }
            compose.runOnIdle { assertEquals(0f,zoom,0f) }
        } finally { loader.shutdown() }
    }

    @Test fun heldPointerKeepsZoomingThroughOverlayAndReleaseClosesIt() {
        val context=InstrumentationRegistry.getInstrumentation().targetContext
        val loader=ImageLoader(context)
        val bitmap=Bitmap.createBitmap(600,300,Bitmap.Config.ARGB_8888)
        var held by mutableStateOf<Long?>(null);var zoom by mutableFloatStateOf(0f);var edits=0
        var dismissed by mutableStateOf(false)
        try {
            compose.setContent { MaterialTheme { Box(Modifier.fillMaxSize()) {
                val distance=with(LocalDensity.current) { 240.dp.toPx() }
                val drag: (Float)->Unit={zoom=(zoom-it/distance).coerceIn(0f,1f)}
                PersonSwipeArea(1,held==null,{edits++},Modifier.fillMaxSize()) {
                    FaceGrid(Person(1,"",1,1,10,listOf(10)),held==null,null,{_,_->null},{edits++},
                        {held=it;dismissed=false},{},onZoomDrag=drag)
                }
                if(held!=null && !dismissed) OriginalPhoto(bitmap,loader,FaceBounds(.2f,.2f,.3f,.3f),zoom,{zoom=it},drag,
                    Modifier.fillMaxSize(),onNewTouch={dismissed=true})
            } } }
            val tile=compose.onNodeWithTag("face-10")
            tile.performTouchInput {down(center)}
            compose.mainClock.advanceTimeBy(800)
            compose.waitUntil(5000) { compose.onAllNodesWithTag("original-face-box").fetchSemanticsNodes().size==1 }
            tile.performTouchInput {moveBy(androidx.compose.ui.geometry.Offset(0f,-150f))}
            compose.runOnIdle {assertEquals(10L,held);assertTrue(zoom>0f);assertEquals(0,edits)}
            tile.performTouchInput {up()}
            compose.runOnIdle {assertNull(held);assertEquals(0,edits)}
            compose.onNodeWithTag("original-photo").assertDoesNotExist()
            tile.performTouchInput {down(center)}
            compose.mainClock.advanceTimeBy(800)
            compose.runOnIdle {assertEquals(10L,held)}
            tile.performTouchInput {down(1,center+androidx.compose.ui.geometry.Offset(20f,0f))}
            compose.onNodeWithTag("original-photo").assertDoesNotExist()
            tile.performTouchInput {up(1);moveBy(androidx.compose.ui.geometry.Offset(0f,-400f));up(0)}
            compose.runOnIdle {assertNull(held);assertEquals(0,edits)}
        } finally { loader.shutdown() }
    }
}
