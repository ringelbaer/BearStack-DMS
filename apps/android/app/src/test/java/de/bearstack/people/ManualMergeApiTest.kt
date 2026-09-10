package de.bearstack.people

import de.bearstack.people.data.remote.LabelingApi
import de.bearstack.people.text.DescribedFailure
import kotlinx.coroutines.runBlocking
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.*
import org.junit.Test

class ManualMergeApiTest {
    @Test fun groupPagePreservesPortraitAndSendsCursorFilterAndProxyPrefix()=runBlocking {
        val requests=mutableListOf<Request>()
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            requests+=chain.request()
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body("""{"people":[{"id":12,"name":"Anna","revision":8,"count":9999,"face_id":120,
                    "faces":[{"id":120,"original_key":"${"a".repeat(64)}","display_path":"Fotos / A.jpg","bounds":{"x":0.1,"y":0.2,"width":0.3,"height":0.4}}]}],"next":12,"has_next":false}""".toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val page=LabelingApi(client,"https://example.test/bearstack/").mergeGroups(10,50,true)
            assertEquals(8L,page.people.single().revision);assertEquals(9999L,page.people.single().count)
            assertEquals("a".repeat(64),page.people.single().originalKeys[120])
            assertEquals(.3f,page.people.single().faceBounds[120]!!.width,0f)
            assertEquals("/bearstack/api/photos/labeling/v1/groups",requests.single().url.encodedPath)
            assertEquals("10",requests.single().url.queryParameter("after"));assertEquals("50",requests.single().url.queryParameter("upper"))
            assertEquals("1",requests.single().url.queryParameter("include_named"))
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun invalidCursorOrderingAndFilteredNamesAreRejected()=runBlocking {
        val person="""{"id":12,"name":"","revision":1,"count":1,"face_id":120}"""
        for(body in listOf(
            """{"people":[$person],"next":11,"has_next":false}""",
            """{"people":[$person],"next":12,"has_next":true}""",
            """{"people":[$person,$person],"next":12,"has_next":false}""",
            """{"people":[${person.replace("\"name\":\"\"","\"name\":\"Anna\"")}],"next":12,"has_next":false}""")) {
            val client=OkHttpClient.Builder().addInterceptor {chain -> Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1)
                .code(200).message("OK").body(body.toResponseBody("application/json".toMediaType())).build()}.build()
            try {
                try {LabelingApi(client,"https://example.test/").mergeGroups(10,50,false);fail("invalid page accepted")}
                catch(e:Exception) {assertTrue(e is DescribedFailure)}
            } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
        }
    }
}
