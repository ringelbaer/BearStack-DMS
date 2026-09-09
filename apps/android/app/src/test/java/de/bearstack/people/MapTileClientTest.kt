package de.bearstack.people

import de.bearstack.people.photos.mapTileClient
import okhttp3.Cache
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.*
import org.junit.Test
import java.io.IOException
import java.nio.file.Files

class MapTileClientTest {
    private fun checkCache(response: MockResponse, calls: Int, etag: Boolean = false) {
        val directory=Files.createTempDirectory("map-cache").toFile()
        val server=MockWebServer()
        server.start()
        val cache=Cache(directory,1024*1024)
        // Rewrite the public URL to an isolated test server after the production
        // host/credential interceptor. No requests leave the test process.
        val client=mapTileClient(cache).newBuilder().addInterceptor {chain ->
            chain.proceed(chain.request().newBuilder().url(server.url(chain.request().url.encodedPath)).build())
        }.build()
        try {
            server.enqueue(response)
            if(calls>1) server.enqueue(if(etag) MockResponse().setResponseCode(304).setHeader("Cache-Control","max-age=60") else response.clone())
            repeat(2) {
                client.newCall(Request.Builder().url("https://tile.openstreetmap.org/3/4/2.png")
                    .header("Authorization","Bearer test-secret").header("Cookie","account=test-secret").header("Proxy-Authorization","test-secret").build())
                    .execute().use {assertEquals(200,it.code);assertEquals("tile",it.body!!.string())}
            }
            assertEquals(calls,server.requestCount)
            val request=server.takeRequest()
            assertNull(request.getHeader("Authorization"));assertNull(request.getHeader("Cookie"));assertNull(request.getHeader("Proxy-Authorization"))
            assertTrue(request.getHeader("User-Agent")!!.startsWith("BearStackPhotos/"))
            if(etag) assertEquals("\"tile-v1\"",server.takeRequest().getHeader("If-None-Match"))
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll();cache.close();server.shutdown();directory.deleteRecursively()}
    }
    @Test fun freshPublicTilesAreCachedAndCredentialsRemoved() = checkCache(MockResponse().setBody("tile").setHeader("Cache-Control","max-age=60"),1)
    @Test fun missingCacheHeadersUseSevenDayFallback() = checkCache(MockResponse().setBody("tile"),1)
    @Test fun expiredTilesRevalidateWithETag() = checkCache(MockResponse().setBody("tile").setHeader("Cache-Control","max-age=0").setHeader("ETag","\"tile-v1\""),2,true)
    @Test fun noStoreTilesAreNotRetained() = checkCache(MockResponse().setBody("tile").setHeader("Cache-Control","no-store"),2)
    @Test fun otherHostsAndCleartextAreRejected() {
        val directory=Files.createTempDirectory("map-host-test").toFile()
        val cache=Cache(directory,1024)
        val client=mapTileClient(cache)
        try {
            for(url in listOf("https://bearstack.example/tile.png","http://tile.openstreetmap.org/1/1/1.png","https://tile.openstreetmap.org:8443/1/1/1.png")) {
                try {client.newCall(Request.Builder().url(url).build()).execute().close();fail("invalid host accepted")}
                catch(e: IOException) {assertEquals("Invalid map tile host",e.message)}
            }
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll();cache.close();directory.deleteRecursively()}
    }
}
