package de.bearstack.people

import de.bearstack.people.data.remote.*
import kotlinx.coroutines.runBlocking
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.*
import org.junit.Test

class PhotosApiTest {
    @Test fun mapKeepsProxyPrefixAndEncodesViewportAndSearch() = runBlocking {
        var request: Request?=null
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            request=chain.request()
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body("""{"total":5,"bounds":{"south":1,"west":170,"north":3,"east":-170},"markers":[{"latitude":2,"longitude":179,"count":4},{"latitude":2,"longitude":-179,"count":1,"path":"trip/a & b.jpg"}]}""".toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val data=PhotosApi(client,"https://example.test/proxy/").map(PhotoQuery(path="trip & beach",query="tag:Meer",type="image"),PhotoMapBounds(-10.0,170.0,10.0,-170.0))
            assertEquals(5,data.total);assertEquals(2,data.markers.size)
            assertEquals("",data.markers.first().path);assertEquals("trip/a & b.jpg",data.markers.last().path)
            assertEquals("/proxy/api/photos/v1/map",request!!.url.encodedPath)
            assertEquals("trip & beach",request!!.url.queryParameter("path"));assertEquals("tag:Meer",request!!.url.queryParameter("q"))
            assertEquals("-170.0",request!!.url.queryParameter("east"));assertEquals("image",request!!.url.queryParameter("type"))
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun readerSessionAndAllRequestsKeepProxyPrefixAndEncodePaths() = runBlocking {
        val requests=mutableListOf<Request>()
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            requests+=chain.request()
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body("""{"protocol":1,"instance":"server","dataset":"photos","account":"reader","can_manage_people":false,
                    "settings":{"thumbnail_size":320,"folder_thumbnail_size":240,"preview_size":1280,"large_preview_size":2048,"slideshow_seconds":5,"frame_seconds":8}}"""
                    .toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val api=PhotosApi(client,"https://example.test/bearstack/")
            val session=api.session()
            assertFalse(session.canManagePeople)
            assertTrue(session.scope.contains("reader"))
            assertEquals("/bearstack/api/photos/v1/session",requests.single().url.encodedPath)
            val photo=Photo("https://other.test/a & b.jpg","a & b.jpg","image","image/jpeg","42","2026-09-09T10:00:00Z",null,50,100,100)
            val url=api.original(photo).let {Request.Builder().url(it).build().url}
            assertEquals("example.test",url.host)
            assertEquals("/bearstack/api/photos/v1/media",url.encodedPath)
            assertEquals(photo.path,url.queryParameter("path"))
            assertEquals("42",url.queryParameter("v"))
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun errorsStayTypedAndOversizedResponsesAreRejected() = runBlocking {
        for(oversized in listOf(false,true)) {
            val client=OkHttpClient.Builder().addInterceptor {chain ->
                Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(if(oversized)200 else 403).message("response")
                    .body((if(oversized) " ".repeat(64*1024+1) else """{"code":"forbidden"}""").toResponseBody("application/json".toMediaType())).build()
            }.build()
            try {
                try { PhotosApi(client,"https://example.test/").session();fail("invalid response admitted") }
                catch(e:java.io.IOException) {if(!oversized){assertTrue(e is ApiFailure);assertEquals(403,(e as ApiFailure).status)}}
            } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
        }
    }
}
