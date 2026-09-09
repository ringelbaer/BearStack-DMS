package de.bearstack.people

import de.bearstack.people.data.remote.*
import kotlinx.coroutines.*
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.*
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.io.OutputStream
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

class PhotoDownloadTest {
    private val photo=Photo("album/a.jpg","a.jpg","image","image/jpeg","1","2026-09-09T10:00:00Z",null,1024,100,100)
    private fun client(body: ByteArray, status: Int=200, mime: String="image/jpeg") = OkHttpClient.Builder().addInterceptor { chain ->
        Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(status).message("response")
            .body(body.toResponseBody(mime.toMediaType())).build()
    }.build()
    @Test fun originalsStreamBeyondJsonLimitAndCloseDestination() = runBlocking {
        val data=ByteArray(3*1024*1024) {(it%251).toByte()}
        val client=client(data)
        val closed=AtomicBoolean(false)
        val output=object:ByteArrayOutputStream() {override fun close(){closed.set(true);super.close()}}
        try {
            var received=0L
            val count=PhotosApi(client,"https://example.test/proxy/").download(photo,{output}) {done,_ -> received=done}
            assertEquals(data.size.toLong(),count);assertEquals(count,received)
            assertTrue(closed.get());assertArrayEquals(data,output.toByteArray())
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun failedOrHtmlResponsesNeverOpenDestination() = runBlocking {
        for(status in listOf(200,403,302)) {
            val client=client("login page".toByteArray(),status,"text/html")
            try {
                try {
                    PhotosApi(client,"https://example.test/").download(photo,{fail("destination opened for non-media");ByteArrayOutputStream()}) {_,_->}
                    fail("non-media downloaded")
                } catch(_: IOException) { }
            } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
        }
    }
    @Test fun cancellationWaitsForTheLastBufferAndClosesOutputBeforeReturning() = runBlocking {
        val client=client(ByteArray(1024*1024))
        val entered=CountDownLatch(1)
        val closed=AtomicBoolean(false)
        val output=object:OutputStream() {
            override fun write(value: Int) {}
            override fun write(buffer: ByteArray,offset: Int,length: Int) {entered.countDown();Thread.sleep(100)}
            override fun close() {closed.set(true)}
        }
        try {
            val task=launch {PhotosApi(client,"https://example.test/").download(photo,{output}) {_,_->}}
            yield()
            assertTrue(withContext(Dispatchers.IO) {entered.await(5,TimeUnit.SECONDS)})
            task.cancelAndJoin()
            assertTrue("output still open after cancellation",closed.get())
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
}
