package de.bearstack.people

import de.bearstack.people.data.remote.ApiFailure
import de.bearstack.people.data.remote.LabelingApi
import kotlinx.coroutines.runBlocking
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.*
import org.junit.Test

class MergeApiTest {
    @Test fun individualSupportAndNextExclusionAreExplicitAndReadOnly() = runBlocking {
        val paths=mutableListOf<String>()
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            paths+=chain.request().url.encodedPath+"?"+chain.request().url.encodedQuery.orEmpty()
            val body=if(chain.request().url.encodedPath.endsWith("/session"))
                """{"protocol":1,"can_manage":true,"instance":"i","dataset":"d","account":"a","upper_id":2,"merge_side_actions":true}"""
            else """{"suggestion":null}"""
            assertEquals("GET",chain.request().method)
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body(body.toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val api=LabelingApi(client,"https://example.test/")
            assertTrue(api.session().mergeSideActions)
            assertNull(api.nextMergeSuggestion(3L to 4L))
            assertEquals("/api/photos/labeling/v1/merge-suggestions/next?exclude_source=3&exclude_target=4",paths.last())
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun nextPairPreservesWitnessMetadataProxyPrefixAndEmptyState() = runBlocking {
        val key="a".repeat(64)
        val responses=ArrayDeque(listOf("""{"suggestion":{"id":8,"score":0.5234,
            "source":{"id":2,"name":"","revision":7,"count":5000,"face_id":9000,"faces":[{"id":9000,"original_key":"$key","display_path":"Fotos / Urlaub / A.jpg","bounds":{"x":0.1,"y":0.2,"width":0.3,"height":0.4}}]},
            "target":{"id":3,"name":"Anna","revision":9,"count":30,"face_id":11,"faces":[{"id":11,"display_path":"Fotos / B.jpg"}]}}}""",
            """{"suggestion":null}"""))
        val paths=mutableListOf<String>()
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            paths+=chain.request().url.encodedPath
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body(responses.removeFirst().toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val api=LabelingApi(client,"https://example.test/bearstack/")
            val pair=api.nextMergeSuggestion()!!
            assertEquals(0.5234,pair.score!!,0.0)
            assertEquals(8L,pair.id);assertEquals(5000L,pair.source.count)
            assertEquals(listOf(9000L),pair.source.faces)
            assertEquals("Anna",pair.target.name);assertEquals(9L,pair.target.revision)
            assertEquals(key,pair.source.originalKeys[9000])
            assertEquals("Fotos / Urlaub / A.jpg",pair.source.facePaths[9000])
            assertEquals(.3f,pair.source.faceBounds.getValue(9000).width,0f)
            assertNull(api.nextMergeSuggestion())
            assertEquals(List(2) {"/bearstack/api/photos/labeling/v1/merge-suggestions/next"},paths)
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }

    @Test fun olderServerScoreIsAbsent() = runBlocking {
        val body="""{"suggestion":{"id":1,"source":{"id":2,"name":"","revision":1,"count":1,"face_id":2,"faces":[]},"target":{"id":3,"name":"","revision":1,"count":1,"face_id":3,"faces":[]}}}"""
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body(body.toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {assertNull(LabelingApi(client,"https://example.test/").nextMergeSuggestion()!!.score)}
        finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }

    @Test fun mergeNamingCapabilityRequiresExplicitServerSupport() = runBlocking {
        for(supported in listOf(false,true)) {
            val body="""{"protocol":1,"can_manage":true,"instance":"i","dataset":"d","account":"a","upper_id":2,"merge_suggestions":true${if(supported) ",\"merge_naming\":true" else ""}}"""
            val client=OkHttpClient.Builder().addInterceptor {chain ->
                Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                    .body(body.toResponseBody("application/json".toMediaType())).build()
            }.build()
            try {assertEquals(supported,LabelingApi(client,"https://example.test/").session().mergeNaming)}
            finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
        }
    }

    @Test fun conflictRemainsTypedForFreshDecision() = runBlocking {
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(409).message("Conflict")
                .body("""{"code":"conflict","error":"changed"}""".toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            try {LabelingApi(client,"https://example.test/").action(3,"{}");fail("conflict accepted")}
            catch(e:ApiFailure) {assertEquals(409,e.status);assertEquals("conflict",e.code)}
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
}
