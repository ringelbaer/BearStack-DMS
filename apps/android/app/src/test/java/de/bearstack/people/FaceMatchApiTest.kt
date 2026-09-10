package de.bearstack.people

import de.bearstack.people.data.remote.*
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.SocketPolicy
import org.junit.Assert.*
import org.junit.Test
import java.util.concurrent.TimeUnit

class FaceMatchApiTest {
    private val anna = """{"id":9,"name":"Änne","count":7,"face_id":901}"""
    private val berta = """{"id":4,"name":"Berta","count":2,"face_id":42}"""
    private fun event(people: String, done: Boolean) = """{"people":[$people],"has_next":false,"done":$done}""" + "\n"

    @Test fun streamingPublishesBeforeCompletionAndReplacesTheRanking() = runBlocking {
        val server=MockWebServer();server.start()
        // A progressing stream must survive the normal overall request timeout.
        val client=localTransport().newBuilder().callTimeout(250,TimeUnit.MILLISECONDS).build()
        try {
            val first=event(anna,false)
            server.enqueue(MockResponse().setHeader("Content-Type","application/x-ndjson; charset=utf-8")
                .setBody(first+event("$berta,$anna",true))
                .throttleBody(first.toByteArray(Charsets.UTF_8).size.toLong(),600,TimeUnit.MILLISECONDS))
            val early=CompletableDeferred<List<FaceMatch>>()
            val search=async {
                LabelingApi(client,server.url("/").toString().replace("http:","https:")).faceMatches(17)
                    .onEach {early.complete(it)}.toList()
            }
            assertEquals(listOf(9L),withTimeout(5000){early.await()}.map {it.id})
            assertFalse("Stream was buffered until the end",search.isCompleted)
            val updates=withTimeout(5000){search.await()}
            assertEquals(2,updates.size)
            assertEquals(listOf(4L,9L),updates.last().map {it.id})
            assertEquals("Änne",updates.last().last().name)
            assertEquals(901L,updates.last().last().faceId)
            assertEquals("application/x-ndjson",server.takeRequest().getHeader("Accept"))
            assertEquals(1,server.requestCount)
        } finally {server.shutdown();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }

    @Test fun fragmentedUtf8AndEmptyFinalRankingAreAccepted() = runBlocking {
        val server=MockWebServer();server.start();val client=localTransport()
        try {
            val api=LabelingApi(client,server.url("/").toString().replace("http:","https:"))
            server.enqueue(MockResponse().setHeader("Content-Type","application/x-ndjson")
                .setChunkedBody("\n"+event(anna,true).replace("\n","\r\n"),1))
            assertEquals("Änne",api.faceMatches(17).single().single().name)
            val first=event(anna,false)
            server.enqueue(MockResponse().setHeader("Content-Type","application/x-ndjson")
                .setBody(first+event("",true)).throttleBody(first.toByteArray().size.toLong(),300,TimeUnit.MILLISECONDS))
            val updates=api.faceMatches(17).toList()
            assertEquals(2,updates.size);assertTrue(updates.last().isEmpty())
        } finally {server.shutdown();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }

    @Test fun invalidTruncatedAndFailedStreamsAreRejected() = runBlocking {
        val server=MockWebServer();server.start();val client=localTransport()
        try {
            val api=LabelingApi(client,server.url("/").toString().replace("http:","https:"))
            val invalid=listOf("",event(anna,false),event(anna,true).trimEnd(),"not json\n",
                "{\"people\":[]}\n","{\"people\":[],\"done\":\"true\"}\n",
                "{\"done\":true}\n",event("{}",true),event("$anna,$anna",true),
                event(List(21){anna}.joinToString(","),true),event(anna.replace("901","0"),true),
                event("",true).replace("\"done\":true","\"done\":true,\"error\":\"server details\""))
            for(body in invalid) {
                server.enqueue(MockResponse().setHeader("Content-Type","application/x-ndjson").setBody(body))
                try {api.faceMatches(1).collect();fail("accepted invalid stream: $body")}
                catch(e:Exception) {assertTrue(e is de.bearstack.people.text.DescribedFailure)}
            }
            for(type in listOf("application/x-ndjson","application/json")) {
                server.enqueue(MockResponse().setHeader("Content-Type",type).setBody(" ".repeat(64*1024+1)+"\n"))
                try {api.faceMatches(1).collect();fail("accepted oversized $type")}
                catch(e:de.bearstack.people.text.UserIoFailure) {assertEquals(R.string.error_response_size,e.userText.resource)}
            }
        } finally {server.shutdown();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }

    @Test fun cancellationAfterAnInterimRankingReleasesTheBlockedReader() = runBlocking {
        val server=MockWebServer();server.start();val client=localTransport()
        try {
            val first=event(anna,false)
            server.enqueue(MockResponse().setHeader("Content-Type","application/x-ndjson")
                .setBody(first+event(berta,true)).throttleBody(first.toByteArray().size.toLong(),2,TimeUnit.SECONDS))
            val early=CompletableDeferred<Unit>()
            val search=launch {
                LabelingApi(client,server.url("/").toString().replace("http:","https:")).faceMatches(1)
                    .collect {early.complete(Unit)}
            }
            withTimeout(5000){early.await()}
            search.cancelAndJoin()
            withTimeout(1000){while(client.dispatcher.runningCallsCount()!=0)delay(10)}
            assertEquals(1,server.requestCount)
        } finally {server.shutdown();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }

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
            val matches=LabelingApi(client,server.url("/bearstack/").toString().replace("http:","https:")).faceMatches(17).single()
            assertEquals(listOf(9L,4L),matches.map {it.id})
            assertEquals(901L,matches.first().faceId);assertEquals(7L,matches.first().count)
            val request=server.takeRequest()
            assertEquals("/bearstack/photos/faces/17/suggestions",request.path)
            assertEquals("GET",request.method);assertEquals("application/x-ndjson",request.getHeader("Accept"))
            assertEquals(1,server.requestCount)
            server.enqueue(MockResponse().setBody("""{"people":[],"has_next":false}"""))
            assertTrue(LabelingApi(client,server.url("/").toString().replace("http:","https:")).faceMatches(17).single().isEmpty())
        } finally {server.shutdown();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun errorsAndOversizedRankingsAreNotAcceptedAsMatches() = runBlocking {
        val server=MockWebServer();server.start();val client=localTransport()
        val api=LabelingApi(client,server.url("/").toString().replace("http:","https:"))
        try {
            for(status in listOf(401,403,404,409,503)) {
                server.enqueue(MockResponse().setResponseCode(status).setBody("{}"))
                try {api.faceMatches(1).collect();fail("accepted HTTP $status")}
                catch(e:ApiFailure){assertEquals(status,e.status)}
            }
            server.enqueue(MockResponse().setBody("{\"people\":["+List(21){"{\"id\":9,\"name\":\"Anna\",\"count\":1,\"face_id\":90}"}.joinToString(",")+"]}"))
            try {api.faceMatches(1).collect();fail("accepted oversized ranking")}
            catch(_:de.bearstack.people.text.UserInputFailure) {}
        } finally {server.shutdown();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun cancellationReleasesTheSearchRequest() = runBlocking {
        val server=MockWebServer();server.start();val client=localTransport()
        try {
            server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.NO_RESPONSE))
            val search=launch(Dispatchers.Default) {LabelingApi(client,server.url("/").toString().replace("http:","https:")).faceMatches(1).collect()}
            assertNotNull(withContext(Dispatchers.IO){server.takeRequest(5,TimeUnit.SECONDS)})
            search.cancelAndJoin()
            withTimeout(5000) {while(client.dispatcher.runningCallsCount()!=0)delay(10)}
            assertEquals(1,server.requestCount)
        } finally {server.shutdown();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
}
