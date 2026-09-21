package de.bearstack.people

import android.content.pm.ActivityInfo
import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Color
import androidx.activity.ComponentActivity
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.test.platform.app.InstrumentationRegistry
import coil.ImageLoader
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.*
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.File

class PhotoViewerInsetsTest {
    @get:Rule val compose=createAndroidComposeRule<ComponentActivity>()
    @Test fun portraitImageCannotShowThroughSystemBarInsets() = checkInsets(false)
    @Test fun landscapeImageCannotShowThroughSystemBarInsets() = checkInsets(true)

    private fun checkInsets(landscape: Boolean) {
        compose.activityRule.scenario.onActivity {it.requestedOrientation=if(landscape) ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE else ActivityInfo.SCREEN_ORIENTATION_PORTRAIT}
        compose.waitUntil(10_000) {compose.activity.resources.configuration.orientation==
            if(landscape) Configuration.ORIENTATION_LANDSCAPE else Configuration.ORIENTATION_PORTRAIT}
        val context=InstrumentationRegistry.getInstrumentation().targetContext
        val file=File(context.cacheDir,"insets-photo.png")
        Bitmap.createBitmap(80,180,Bitmap.Config.ARGB_8888).apply {
            eraseColor(Color.GREEN)
            file.outputStream().use {compress(Bitmap.CompressFormat.PNG,100,it)}
            recycle()
        }
        val photo=shareTestPhoto.copy(path=file.toURI().toString(),mime="image/png",width=80,height=180)
        val service=object:ShareTestService() {
            override suspend fun browse(query:PhotoQuery,page:Int,section:String)=
                PhotoPage("","",1,1,false,0,false,false,listOf(photo),emptyList(),emptyList())
            override fun thumbnail(photo:Photo,size:Int)=file.toURI().toString()
            override fun original(photo:Photo)=file.toURI().toString()
        }
        val images=ImageLoader.Builder(context).build()
        val owner=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate)
        lateinit var controller:PhotosController
        compose.runOnUiThread {controller=PhotosController(owner,service,DevicePhotosService.SESSION)}
        try {
            compose.setGermanContent {PhotoViewer(controller,images,listOf(photo),photo.path)}
            compose.waitUntil(10_000) {compose.onAllNodesWithTag("photo-viewer-image").fetchSemanticsNodes().any {
                it.config.getOrElse(SemanticsActions.CustomActions) {emptyList()}.isNotEmpty()
            }}
            val root=compose.onNode(isDialog()).fetchSemanticsNode().boundsInRoot
            val top=compose.onNodeWithTag("photo-viewer-top-bar").fetchSemanticsNode().boundsInRoot
            val bottom=compose.onNodeWithTag("photo-viewer-bottom-bar").fetchSemanticsNode().boundsInRoot
            assertEquals(root.top,top.top,1f)
            assertEquals(root.bottom,bottom.bottom,1f)
            val bitmap=compose.onNode(isDialog()).captureToImage().asAndroidBitmap()
            assertEquals(Color.GREEN,bitmap.getPixel(bitmap.width/2,bitmap.height/2))
            assertEquals(Color.BLACK,bitmap.getPixel(bitmap.width/2,2))
            assertEquals(Color.BLACK,bitmap.getPixel(bitmap.width/2,bitmap.height-3))
            File(context.cacheDir,"viewer-insets-${if(landscape) "landscape" else "portrait"}.png").outputStream().use {
                bitmap.compress(Bitmap.CompressFormat.PNG,100,it)
            }
        } finally {
            compose.runOnUiThread {controller.close();owner.cancel();images.shutdown()}
            file.delete()
            compose.activityRule.scenario.onActivity {it.requestedOrientation=ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED}
        }
    }
}
