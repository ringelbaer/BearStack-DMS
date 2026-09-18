package de.bearstack.people

import android.graphics.Bitmap
import androidx.test.platform.app.InstrumentationRegistry
import coil.ImageLoader
import coil.decode.DataSource
import coil.request.CachePolicy
import coil.request.ImageRequest
import coil.request.ErrorResult
import coil.request.SuccessResult
import de.bearstack.people.data.remote.*
import de.bearstack.people.media.*
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.IOException
import kotlinx.coroutines.*
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.SocketPolicy
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import okio.Buffer
import org.junit.Assert.*
import org.junit.Test

class ThumbnailCacheTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private val session = PhotoSession("account", false, 320, 240, 1280, 2048, 5, 8)
    private fun image(): ByteArray {
        val bitmap = Bitmap.createBitmap(64, 32, Bitmap.Config.ARGB_8888)
        return ByteArrayOutputStream().also { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it); bitmap.recycle() }.toByteArray()
    }
    private val unused = object : PhotosService {
        override suspend fun session() = session
        override suspend fun browse(query: PhotoQuery, page: Int, section: String) = error("unused")
        override suspend fun info(path: String) = error("unused")
        override suspend fun blog(path: String) = error("unused")
        override fun thumbnail(photo: Photo, size: Int) = error("unused")
        override fun original(photo: Photo) = error("unused")
    }

    private suspend fun scenario(block: suspend (MockWebServer, OkHttpClient, File, CoroutineScope) -> Unit) {
        val certificate = HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        val trusted = HandshakeCertificates.Builder().addTrustedCertificate(certificate.certificate).build()
        val client = OkHttpClient.Builder().sslSocketFactory(trusted.sslSocketFactory(), trusted.trustManager).build()
        val server = MockWebServer().apply {
            useHttps(HandshakeCertificates.Builder().heldCertificate(certificate).build().sslSocketFactory(), false); start()
        }
        val root = File(context.cacheDir, "thumbnail-test-${System.nanoTime()}")
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        try { block(server, client, root, scope) }
        finally { scope.cancel(); client.dispatcher.cancelAll(); server.shutdown(); root.deleteRecursively() }
    }

    @Test fun parallelLoadsCoalesceAndCoilUsesDiskAfterMemoryAndProcessRestart() = runBlocking {
        scenario { server, client, directory, scope ->
            val data = CachedThumbnail(server.url("/thumbnail?size=320").toString(), "account:1")
            val disk = ThumbnailDiskCache(directory, MIB)
            disk.protect(setOf(data.key))
            var cache = ThumbnailCache(disk, client, unused, session, scope)
            server.enqueue(MockResponse().setHeader("Content-Type", "image/png").setBody(Buffer().write(image())))
            val results = coroutineScope { List(10) { async { cache.load(data) } }.awaitAll() }
            assertEquals(1, server.requestCount)
            assertEquals(1, results.count { it.second == DataSource.NETWORK })
            cache.close()
            cache = ThumbnailCache(ThumbnailDiskCache(directory, 1), client, unused, session, scope)
            val loader = ImageLoader.Builder(context).diskCachePolicy(CachePolicy.DISABLED)
                .components { add(ThumbnailCache.Factory(cache)); add(ThumbnailCache.Keys()) }.build()
            try {
                val request = ImageRequest.Builder(context).data(data).size(64, 32).build()
                assertEquals(DataSource.DISK, (loader.execute(request) as SuccessResult).dataSource)
                assertEquals(DataSource.MEMORY_CACHE, (loader.execute(request) as SuccessResult).dataSource)
                loader.memoryCache!!.clear()
                assertEquals(DataSource.DISK, (loader.execute(request) as SuccessResult).dataSource)
                assertEquals(1, server.requestCount)
                assertEquals(1, cache.state.value.usage.pinnedEntries)
            } finally { loader.shutdown(); cache.close(clear = true) }
            assertFalse(directory.exists())
        }
    }

    @Test fun errorsOversizedResponsesAndInvalidImagesNeverPopulateCache() = runBlocking {
        scenario { server, client, directory, scope ->
            val disk = ThumbnailDiskCache(directory, MIB)
            val cache = ThumbnailCache(disk, client, unused, session, scope)
            val data = CachedThumbnail(server.url("/thumbnail").toString(), "1")
            val responses = listOf(MockResponse().setResponseCode(401),
                MockResponse().setHeader("Content-Type", "text/html").setBody("error"),
                MockResponse().setHeader("Content-Type", "image/png").setBody("broken image"),
                MockResponse().setHeader("Content-Type", "image/png").setBody(Buffer().write(ByteArray(THUMBNAIL_MAX_BYTES.toInt() + 1))))
            try {
                for (response in responses) {
                    server.enqueue(response)
                    try { cache.load(data); fail("invalid thumbnail accepted") } catch (_: IOException) { }
                    assertEquals(0L, disk.usage().bytes)
                }
                server.enqueue(MockResponse().setHeader("Content-Type", "image/png").setBody(Buffer().write(image())))
                assertEquals(DataSource.NETWORK, cache.load(data).second)
                assertEquals(5, server.requestCount)
            } finally { cache.close() }
        }
    }

    @Test fun cancellingCallerAndClosingSessionCancelInFlightDownloadsWithoutLateWrites() = runBlocking {
        scenario { server, client, directory, scope ->
            val disk = ThumbnailDiskCache(directory, MIB)
            val cache = ThumbnailCache(disk, client, unused, session, scope)
            val data = CachedThumbnail(server.url("/thumbnail").toString(), "1")
            server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.NO_RESPONSE))
            val load = async { cache.load(data) }
            withTimeout(5000) { while (server.requestCount < 1) delay(10) }
            load.cancelAndJoin()
            withTimeout(5000) { cache.close(clear = true) }
            assertFalse(directory.exists())
        }
    }

    @Test fun warmerPinsAllRequiredImagesAndFailedMetadataRetainsPreviousManifest() = runBlocking {
        scenario { server, client, directory, scope ->
            var fail = false
            val metadataCalls = java.util.concurrent.atomic.AtomicInteger()
            val service = object : PhotosService by unused {
                override suspend fun browse(query: PhotoQuery, page: Int, section: String): PhotoPage {
                    metadataCalls.incrementAndGet()
                    if (fail) throw IOException("offline")
                    val photo = Photo("photo", "photo", "image", "image/png", "1", "2026-09-17", null, 100, 64, 32)
                    return PhotoPage("", "", 1, 1, false, 1, false, false,
                        if (query.recursive) listOf(photo) else emptyList(),
                        if (query.recursive) emptyList() else listOf(PhotoFolder("folder", "folder", null, 1, false, 0, listOf(photo))), emptyList())
                }
                override fun thumbnail(photo: Photo, size: Int) = server.url("/thumbnail?size=$size").toString()
            }
            repeat(2) { server.enqueue(MockResponse().setHeader("Content-Type", "image/png").setBody(Buffer().write(image()))) }
            val disk = ThumbnailDiskCache(directory, 1)
            val cache = ThumbnailCache(disk, client, service, session, scope)
            try {
                cache.refresh(force = true)
                withTimeout(5000) { while (cache.state.value.usage.pinnedEntries != 2 || cache.state.value.syncing) delay(10) }
                assertEquals(2, cache.state.value.usage.requiredEntries)
                assertTrue(cache.state.value.usage.bytes > 1)
                // A second warm pass must neither download nor touch image/pin files.
                val files = directory.listFiles()!!.filter { it.isFile }
                files.forEach { assertTrue(it.setLastModified(1000)) }
                cache.refresh(force = true)
                withTimeout(5000) { while (metadataCalls.get() < 4 || cache.state.value.syncing) delay(10) }
                assertEquals(2, server.requestCount)
                files.forEach { assertEquals(1000L, it.lastModified()) }
                val missing = files.first { it.name != "pins" }
                assertTrue(missing.delete())
                server.enqueue(MockResponse().setHeader("Content-Type", "image/png").setBody(Buffer().write(image())))
                cache.refresh(force = true)
                withTimeout(5000) { while (metadataCalls.get() < 6 || cache.state.value.syncing) delay(10) }
                assertTrue(missing.isFile)
                assertEquals(3, server.requestCount)
                fail = true
                cache.refresh(force = true)
                withTimeout(5000) { while (!cache.state.value.failed || cache.state.value.syncing) delay(10) }
                assertEquals(2, disk.usage().pinnedEntries)
                assertEquals(3, server.requestCount)
            } finally { cache.close() }
        }
    }

    @Test fun undecodableDiskEntryIsRemovedSoTheNextDisplayCanRecover() = runBlocking {
        scenario { server, client, directory, scope ->
            val disk = ThumbnailDiskCache(directory, MIB)
            val data = CachedThumbnail(server.url("/thumbnail").toString(), "1")
            disk.write(data.key, "corrupt encoded data".toByteArray())
            val cache = ThumbnailCache(disk, client, unused, session, scope)
            val loader = ImageLoader.Builder(context).diskCachePolicy(CachePolicy.DISABLED).components {
                add(ThumbnailCache.Factory(cache)); add(ThumbnailCache.Keys()); add(ThumbnailCache.Integrity(cache))
            }.build()
            try {
                val request = ImageRequest.Builder(context).data(data).size(64, 32).build()
                assertTrue(loader.execute(request) is ErrorResult)
                assertEquals(0L, disk.usage().bytes)
                server.enqueue(MockResponse().setHeader("Content-Type", "image/png").setBody(Buffer().write(image())))
                assertEquals(DataSource.NETWORK, (loader.execute(request) as SuccessResult).dataSource)
                assertEquals(1, server.requestCount)
            } finally { loader.shutdown(); cache.close() }
        }
    }
}
