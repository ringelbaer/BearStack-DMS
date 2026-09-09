package de.bearstack.people

import android.content.pm.ActivityInfo
import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Path
import android.graphics.drawable.AdaptiveIconDrawable
import androidx.activity.ComponentActivity
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.platform.*
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.unit.Density
import androidx.test.platform.app.InstrumentationRegistry
import coil.ImageLoader
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.*
import de.bearstack.people.ui.BearStackTheme
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.File
import java.util.Locale

class PhotoLayoutTest {
    @get:Rule val compose=createAndroidComposeRule<ComponentActivity>()

    @Test fun germanLightPortrait()=gallery(Locale.GERMAN,false,false,1f)
    @Test fun englishDarkLandscape()=gallery(Locale.ENGLISH,true,true,1f)
    @Test fun germanLargeFontPortrait()=gallery(Locale.GERMAN,true,false,2f)

    private fun gallery(locale: Locale,dark: Boolean,landscape: Boolean,scale: Float) {
        val instrumentation=InstrumentationRegistry.getInstrumentation()
        val app=instrumentation.targetContext
        compose.activityRule.scenario.onActivity {it.requestedOrientation=if(landscape) ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE else ActivityInfo.SCREEN_ORIENTATION_PORTRAIT}
        compose.waitUntil(10_000) {compose.activity.resources.configuration.orientation==if(landscape) Configuration.ORIENTATION_LANDSCAPE else Configuration.ORIENTATION_PORTRAIT}
        val photoFile=File(app.cacheDir,"layout-photo.webp")
        instrumentation.context.assets.open("bearstack-hero-desk.webp").use {input -> photoFile.outputStream().use(input::copyTo)}
        val images=ImageLoader.Builder(app).build()
        val owner=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate)
        val photos=(1..60).map {Photo("Holiday/$it.webp","Photo $it","image","image/webp","1",
            if(it<10) "2026-09-09T10:00:00Z" else "2026-09-08T10:00:00Z",null,photoFile.length(),1717,916)}
        val api=object:PhotosService {
            override suspend fun session()=PhotoSession("visual",false,240,240,1280,2048,5,8)
            override suspend fun browse(query: PhotoQuery,page: Int,section: String)=PhotoPage(query.path,"",1,photos.size,false,
                if(query.recursive) 0 else 2,false,false,if(query.recursive) photos else emptyList(),
                if(query.recursive) emptyList() else listOf("Holiday","A longer album title with memories").map {PhotoFolder(it,it,null,60,false,0,photos.take(2))},emptyList())
            override suspend fun info(path: String)=photos.first {it.path==path}.copy(camera="BearStack Camera",lens="35 mm",rating=4.5)
            override suspend fun blog(path: String)=error("not used")
            override fun thumbnail(photo: Photo,size: Int)=photoFile.toURI().toString()
            override fun original(photo: Photo)=photoFile.toURI().toString()
        }
        lateinit var controller:PhotosController
        compose.runOnUiThread {controller=PhotosController(owner,api,PhotoSession("visual",false,240,240,1280,2048,5,8))}
        val name="${locale.language}-${if(dark) "dark" else "light"}-${if(landscape) "landscape" else "portrait"}-${scale.toInt()}"
        fun save(part: String,node: SemanticsNodeInteraction=compose.onRoot()) {
            node.captureToImage().asAndroidBitmap().let {bitmap ->
                File(app.cacheDir,"layout-$name-$part.png").outputStream().use {bitmap.compress(Bitmap.CompressFormat.PNG,100,it)}
            }
        }
        try {
            compose.setContent {
                val base=LocalContext.current
                val config=Configuration(LocalConfiguration.current).apply {setLocale(locale);fontScale=scale}
                val context=base.createConfigurationContext(config)
                CompositionLocalProvider(LocalContext provides context,LocalResources provides context.resources,
                    LocalConfiguration provides config,LocalDensity provides Density(LocalDensity.current.density,scale)) {
                    BearStackTheme(dark) {PhotosScreen(controller,images,false,{fail("reader reached editing")},{})}
                }
            }
            compose.waitUntil(10_000) {!controller.state.value.loading && images.memoryCache?.keys?.isNotEmpty()==true}
            val german=locale==Locale.GERMAN
            compose.onNodeWithContentDescription("Photo 1").assertIsDisplayed()
            compose.onNodeWithText(if(german) "Fotos" else "Photos").assertIsDisplayed()
            save("grid")
            compose.onNodeWithContentDescription("Photo 1").performClick()
            compose.onNodeWithContentDescription(if(german) "Informationen" else "Information").assertIsDisplayed().performClick()
            compose.waitUntil(10_000) {compose.onAllNodesWithText("BearStack Camera",substring=true).fetchSemanticsNodes().isNotEmpty()}
            save("info",compose.onNode(isDialog() and hasAnyDescendant(hasText("BearStack Camera",substring=true))))
            compose.runOnUiThread {controller.closeViewer()}
            compose.onNodeWithText(if(german) "Ordner" else "Folders").performClick()
            compose.onNodeWithText("Holiday").assertIsDisplayed()
            save("folders")
            compose.onNodeWithText("Holiday").performClick()
            assertEquals("Holiday",controller.state.value.query.path)
            compose.onNodeWithContentDescription(if(german) "Zurück" else "Back").performClick()
            compose.waitUntil(5_000) {!controller.state.value.loading && controller.state.value.query.path.isEmpty()}
        } finally {
            compose.runOnUiThread {controller.close();owner.cancel();images.shutdown()}
            photoFile.delete()
            compose.activityRule.scenario.onActivity {it.requestedOrientation=ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED}
        }
    }

    @Test fun launcherArtworkFitsRoundMask() {
        val app=InstrumentationRegistry.getInstrumentation().targetContext
        val icon=app.getDrawable(R.mipmap.ic_launcher) as AdaptiveIconDrawable
        val image=Bitmap.createBitmap(512,512,Bitmap.Config.ARGB_8888)
        val canvas=Canvas(image)
        icon.setBounds(0,0,512,512)
        canvas.clipPath(Path().apply {addCircle(256f,256f,256f,Path.Direction.CW)})
        icon.draw(canvas)
        val foreground=Bitmap.createBitmap(512,512,Bitmap.Config.ARGB_8888)
        icon.foreground.draw(Canvas(foreground))
        var painted=0
        for(y in 0 until 512) for(x in 0 until 512) {
            if((foreground.getPixel(x,y) ushr 24)>32) {
                painted++
                assertTrue("Artwork cropped at $x/$y",(x-256)*(x-256)+(y-256)*(y-256)<256*256)
            }
        }
        assertTrue("Empty launcher artwork",painted>1000)
        File(app.cacheDir,"layout-launcher-round.png").outputStream().use {image.compress(Bitmap.CompressFormat.PNG,100,it)}
        foreground.recycle();image.recycle()
    }
}
