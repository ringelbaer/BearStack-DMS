package de.bearstack.people

import de.bearstack.people.data.remote.*
import kotlinx.coroutines.*
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.SocketPolicy
import org.junit.Assert.*
import org.junit.Test
import java.util.concurrent.TimeUnit

class FaceMatchApiTest {
    // Production addresses remain HTTPS-only. The test transport sends the
    // request to MockWebServer's local HTTP listener after URL validation.
    private fun localTransport() = OkHttpClient.Builder().addInterceptor { chain ->
        chain.proceed(chain.request().newBuilder().url(chain.request().url.newBuilder().scheme("http").build()).build())
    }.build()
    @Test fun rankingPreservesOrderWitnessPortraitsAndProxyPrefix() = runBlocking {
        val server=MockWebServer();server.start()
        val client=localTransport()
        try {
            server.enqueue(MockResponse().setBody("""{"people":[{"id":9,"name":"Anna","count":7,"face_id":901},{"id":4,"name":"Berta","count":2,"face_id":42}],"has_next":false}"""))
            val matches=LabelingApi(client,server.url("/bearstack/").toString().replace("http:","https:")).faceMatches(17)
            assertEquals(listOf(9L,4L),matches.map {it.id})
            assertEquals(901L,matches.first().faceId);assertEquals(7L,matches.first().count)
            val request=server.takeRequest()
            assertEquals("/bearstack/photos/faces/17/suggestions",request.path)
            assertEquals("GET",request.method);assertEquals("application/json",request.getHeader("Accept"))
            assertEquals(1,server.requestCount)
            server.enqueue(MockResponse().setBody("""{"people":[],"has_next":false}"""))
            assertTrue(LabelingApi(client,server.url("/").toString().replace("http:","https:")).faceMatches(17).isEmpty())
        } finally {server.shutdown();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun errorsAndOversizedRankingsAreNotAcceptedAsMatches() = runBlocking {
        val server=MockWebServer();server.start();val client=localTransport()
        val api=LabelingApi(client,server.url("/").toString().replace("http:","https:"))
        try {
            for(status in listOf(401,403,404,409,503)) {
                server.enqueue(MockResponse().setResponseCode(status).setBody("{}"))
                try {api.faceMatches(1);fail("accepted HTTP $status")}
                catch(e:ApiFailure){assertEquals(status,e.status)}
            }
            server.enqueue(MockResponse().setBody("{\"people\":["+List(21){"{\"id\":9,\"name\":\"Anna\",\"count\":1,\"face_id\":90}"}.joinToString(",")+"]}"))
            try {api.faceMatches(1);fail("accepted oversized ranking")}
            catch(_:de.bearstack.people.text.UserInputFailure) {}
        } finally {server.shutdown();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun cancellationReleasesTheSearchRequest() = runBlocking {
        val server=MockWebServer();server.start();val client=localTransport()
        try {
            server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.NO_RESPONSE))
            val search=launch(Dispatchers.Default) {LabelingApi(client,server.url("/").toString().replace("http:","https:")).faceMatches(1)}
            assertNotNull(withContext(Dispatchers.IO){server.takeRequest(5,TimeUnit.SECONDS)})
            search.cancelAndJoin()
            withTimeout(5000) {while(client.dispatcher.runningCallsCount()!=0)delay(10)}
            assertEquals(1,server.requestCount)
        } finally {server.shutdown();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
}
