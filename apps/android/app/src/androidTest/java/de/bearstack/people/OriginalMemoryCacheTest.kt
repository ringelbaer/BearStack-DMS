package de.bearstack.people

import android.graphics.Bitmap
import android.graphics.drawable.BitmapDrawable
import androidx.test.platform.app.InstrumentationRegistry
import coil.ImageLoader
import coil.decode.DataSource
import coil.memory.MemoryCache
import coil.request.CachePolicy
import coil.request.ErrorResult
import coil.request.SuccessResult
import de.bearstack.people.people.*
import de.bearstack.people.ui.originalPhotoRequest
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import okio.Buffer
import org.junit.Assert.*
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.util.concurrent.atomic.AtomicLong

class OriginalMemoryCacheTest {
    @Test fun differentFacesReuseDecodedOriginalUntilFixedDeadlineAndClearForNewConnection() = runBlocking {
        val context=InstrumentationRegistry.getInstrumentation().targetContext
        val certificate=HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        val serverCertificates=HandshakeCertificates.Builder().heldCertificate(certificate).build()
        val clientCertificates=HandshakeCertificates.Builder().addTrustedCertificate(certificate.certificate).build()
        val client=OkHttpClient.Builder().sslSocketFactory(clientCertificates.sslSocketFactory(),clientCertificates.trustManager).build()
        val server=MockWebServer().apply {useHttps(serverCertificates.sslSocketFactory(),false);start()}
        val now=AtomicLong(0)
        val cache=OriginalMemoryCache(MemoryCache.Builder(context).maxSizeBytes(16*1024*1024).weakReferencesEnabled(false).build(),now::get)
        val loader=ImageLoader.Builder(context).okHttpClient(client).memoryCache(cache).diskCachePolicy(CachePolicy.DISABLED).build()
        val bitmap=Bitmap.createBitmap(64,32,Bitmap.Config.ARGB_8888)
        val data=ByteArrayOutputStream().also {bitmap.compress(Bitmap.CompressFormat.PNG,100,it)}.toByteArray()
        fun response()=MockResponse().setHeader("Content-Type","image/png").setHeader("Cache-Control","private, no-store").setBody(Buffer().write(data))
        suspend fun load(face: Int, key: String) = loader.execute(originalPhotoRequest(context,server.url("/faces/$face/original").toString(),key))
        try {
            server.enqueue(response())
            val prefetched=CompletableDeferred<SuccessResult>()
            val preloadJob=launch {
                WifiOriginalPreloader(MutableStateFlow(true)) {true}.preload(listOf("account:photo-a")) {
                    prefetched.complete(load(1,it) as SuccessResult)
                }
            }
            val first=withTimeout(5000) {prefetched.await()}
            preloadJob.cancelAndJoin()
            assertEquals(DataSource.NETWORK,first.dataSource)
            now.set(ORIGINAL_CACHE_TTL_MS-1)
            val otherFace=load(2,"account:photo-a") as SuccessResult
            assertEquals(DataSource.MEMORY_CACHE,otherFace.dataSource)
            assertSame((first.drawable as BitmapDrawable).bitmap,(otherFace.drawable as BitmapDrawable).bitmap)
            assertEquals(1,server.requestCount)
            // The preceding hit must not reset the three-minute deadline.
            now.set(ORIGINAL_CACHE_TTL_MS)
            server.enqueue(response())
            assertEquals(DataSource.NETWORK,(load(2,"account:photo-a") as SuccessResult).dataSource)
            assertEquals(2,server.requestCount)
            server.enqueue(response())
            assertEquals(DataSource.NETWORK,(load(2,"account:photo-b") as SuccessResult).dataSource)
            cache.clear()
            server.enqueue(response())
            assertEquals(DataSource.NETWORK,(load(1,"account:photo-a") as SuccessResult).dataSource)
            assertEquals(4,server.requestCount)
            server.enqueue(MockResponse().setResponseCode(403))
            assertTrue(load(3,"account:photo-c") is ErrorResult)
            server.enqueue(response())
            assertEquals(DataSource.NETWORK,(load(3,"account:photo-c") as SuccessResult).dataSource)
            assertEquals(6,server.requestCount)
        } finally {
            loader.shutdown();cache.clear();bitmap.recycle();server.shutdown()
            client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()
        }
    }

    @Test fun boundedCacheDoesNotResurrectEvictedOriginalsAndLegacyUrlsStaySeparate() {
        val context=InstrumentationRegistry.getInstrumentation().targetContext
        var now=0L
        val cache=OriginalMemoryCache(MemoryCache.Builder(context).maxSizeBytes(256).weakReferencesEnabled(false).build(),{now})
        val a=Bitmap.createBitmap(8,8,Bitmap.Config.ARGB_8888)
        val b=Bitmap.createBitmap(8,8,Bitmap.Config.ARGB_8888)
        val first=MemoryCache.Key(ORIGINAL_CACHE_PREFIX+"a")
        val second=MemoryCache.Key(ORIGINAL_CACHE_PREFIX+"b")
        try {
            cache[first]=MemoryCache.Value(a)
            cache[second]=MemoryCache.Value(b)
            assertTrue(cache.size<=256);assertNull(cache[first]);assertSame(b,cache[second]!!.bitmap)
            now=ORIGINAL_CACHE_TTL_MS
            assertNull(cache[second]);assertEquals(0,cache.size)
            val thumbnail=MemoryCache.Key("thumbnail")
            cache[thumbnail]=MemoryCache.Value(a)
            now+=ORIGINAL_CACHE_TTL_MS
            assertSame(a,cache[thumbnail]!!.bitmap)
            val oldA=originalPhotoRequest(context,"https://example.test/faces/1/original")
            val oldB=originalPhotoRequest(context,"https://example.test/faces/2/original")
            assertNotEquals(oldA.memoryCacheKey,oldB.memoryCacheKey)
            assertEquals(CachePolicy.DISABLED,oldA.diskCachePolicy)
        } finally {cache.clear();a.recycle();b.recycle()}
    }
}
