package de.bearstack.people

import android.content.ContentValues
import android.os.Build
import android.provider.MediaStore
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.PhotoDownloads
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import java.io.IOException
import java.io.OutputStream
import java.util.UUID

class PhotoDownloadStorageTest {
    @Test fun successWritesOriginalAndFailureOrCancellationRemoveTheCreatedDocument() = runBlocking {
        assumeTrue(Build.VERSION.SDK_INT>=29)
        val context=InstrumentationRegistry.getInstrumentation().targetContext
        val resolver=context.contentResolver
        val photo=Photo("photo.jpg","photo.jpg","image","image/jpeg","1","2026-09-09T10:00:00Z",null,3,10,10)
        for(mode in listOf("success","failure","cancel")) {
            val uri=resolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI,ContentValues().apply {
                put(MediaStore.Downloads.DISPLAY_NAME,"bearstack-test-${UUID.randomUUID()}.jpg")
                put(MediaStore.Downloads.MIME_TYPE,"image/jpeg")
            })!!
            val scope=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate)
            val entered=CompletableDeferred<Unit>()
            val service=object:PhotosService {
                override suspend fun session():PhotoSession=error("unused")
                override suspend fun browse(query:PhotoQuery,page:Int,section:String):PhotoPage=error("unused")
                override suspend fun info(path:String):Photo=error("unused")
                override suspend fun blog(path:String):PhotoBlog=error("unused")
                override fun thumbnail(photo:Photo,size:Int):String=error("unused")
                override fun original(photo:Photo):String=error("unused")
                override suspend fun download(photo:Photo,destination:()->OutputStream,progress:(Long,Long)->Unit):Long = withContext(Dispatchers.IO) {
                    destination().use {output ->
                        output.write(byteArrayOf(1,2,3));entered.complete(Unit)
                        if(mode=="failure") throw IOException("interrupted transfer")
                        if(mode=="cancel") awaitCancellation()
                        3L
                    }
                }
            }
            try {
                val downloads=PhotoDownloads(context,scope,service)
                withContext(Dispatchers.Main) {downloads.save(photo,uri)}
                withTimeout(5000) {while(!entered.isCompleted && downloads.state.value.active) delay(10)}
                assertTrue("$mode did not start: ${downloads.state.value}",entered.isCompleted)
                if(mode=="cancel") withContext(Dispatchers.Main) {downloads.cancel()}
                withTimeout(5000) {while(downloads.state.value.active) delay(10)}
                if(mode=="success") {
                    assertTrue(downloads.state.value.complete)
                    resolver.openInputStream(uri)!!.use {assertArrayEquals(byteArrayOf(1,2,3),it.readBytes())}
                } else resolver.query(uri,null,null,null,null).use {assertTrue(it==null || it.count==0)}
            } finally {scope.cancel();resolver.delete(uri,null,null)}
        }
    }
}
