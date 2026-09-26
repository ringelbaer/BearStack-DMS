package de.bearstack.people

import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.canGroupSelection
import kotlinx.coroutines.runBlocking
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.*
import org.junit.Test

class ImageGroupsTest {
    private fun photo(path: String,group: Long=0,type: String="image")=Photo(path,path,type,"image/jpeg","1","",null,0,0,0,imageGroupId=group)
    @Test fun selectionRejectsVideosMultipleGroupsAndNoNewImages() {
        assertTrue(canGroupSelection(listOf(photo("a"),photo("b"))))
        assertTrue(canGroupSelection(listOf(photo("a",3),photo("b"))))
        assertFalse(canGroupSelection(listOf(photo("a",3),photo("b",4))))
        assertFalse(canGroupSelection(listOf(photo("a",3),photo("b",3))))
        assertFalse(canGroupSelection(listOf(photo("a"),photo("b",type="video"))))
        assertFalse(canGroupSelection(listOf(photo("a"))))
    }
    @Test fun groupApiKeepsProxyPrefixAndSendsRepeatedEncodedPathsAndRevision()=runBlocking {
        val requests=mutableListOf<Request>()
        var response="""{"id":7}"""
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            requests+=chain.request()
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body(response.toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val api=PhotosApi(client,"https://example.test/proxy/")
            assertEquals(7,api.createImageGroup(listOf("a & b.jpg","other/c.jpg"),"a & b.jpg"))
            assertEquals("/proxy/photos/image-groups",requests.last().url.encodedPath)
            assertEquals("application/json",requests.last().header("Accept"))
            val form=requests.last().body as FormBody
            assertEquals(listOf("a & b.jpg","other/c.jpg"),(0 until form.size).filter {form.name(it)=="ids"}.map {form.value(it)})
            response="""{"id":7,"revision":9,"members":[{"entity_id":12,"path":"a & b.jpg","display_path":"Fotos / a & b.jpg","primary":true,"missing":false}]}"""
            val group=api.imageGroup(7)
            assertEquals(9,group.revision)
            response="""{"exists":false}"""
            assertFalse(api.updateImageGroup(group,"dissolve"))
            assertEquals("/proxy/photos/image-groups/7",requests.last().url.encodedPath)
            val update=requests.last().body as FormBody
            assertEquals("9",update.value(0))
            response="""{"path":"other/c.jpg","directory":"other","page":300}"""
            assertEquals(300,api.locatePhoto("other/c.jpg").page)
            assertEquals("/proxy/api/photos/v1/browse/position",requests.last().url.encodedPath)
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
}
