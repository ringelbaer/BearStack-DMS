package de.bearstack.people

import de.bearstack.people.data.remote.*
import kotlinx.coroutines.runBlocking
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.*
import org.junit.Test

class PersonFoldersApiTest {
    private fun client(body: String, status: Int=200, check: (Request)->Unit = {}) = OkHttpClient.Builder().addInterceptor {chain ->
        check(chain.request())
        Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(status).message("test")
            .body(body.toResponseBody("application/json".toMediaType())).build()
    }.build()
    private fun OkHttpClient.close() {dispatcher.executorService.shutdown();connectionPool.evictAll()}

    @Test fun capabilityIsOptIn() = runBlocking {
        for(supported in listOf(false,true)) {
            val client=client("""{"protocol":1,"can_manage":true,"instance":"i","dataset":"d","account":"a","upper_id":8${if(supported) ",\"person_folders\":true" else ""}}""")
            try {assertEquals(supported,LabelingApi(client,"https://test.invalid/").session().personFolders)} finally {client.close()}
        }
    }
    @Test fun pathsRootExclusionsOriginalIdentityAndBoundsSurviveProxyAndPaging() = runBlocking {
        val key="a".repeat(64)
        val client=client("""{"person_id":7,"name":"Ada","revision":19,"page":2,"has_next":true,"folders":[
          {"directory":"","display_path":"Fotos","count":501,"excluded":false,"face_ids":[8,9],"faces":[
            {"id":8,"display_path":"Fotos / original.jpg","original_key":"$key","bounds":{"x":0.1,"y":0.2,"width":0.3,"height":0.4}},
            {"id":9,"display_path":"Fotos / changed.jpg","needs_review":true,"bounds":{"x":0.1,"y":0.2,"width":0.3,"height":0.4}}]},
          {"directory":"20240102_Family_Trip/Nested_Folder","display_path":"Fotos / 02.01.2024 · Family Trip / Nested Folder","count":0,"excluded":true,"face_ids":[]}] }""") {
            assertEquals("GET",it.method)
            assertEquals("/proxy/api/photos/labeling/v1/people/7/folders?page=2",it.url.encodedPath+"?"+it.url.encodedQuery)
        }
        try {
            val result=LabelingApi(client,"https://test.invalid/proxy/").personFolders(7,2)
            assertTrue(result.hasNext);assertEquals(19L,result.revision)
            val root=result.folders.first()
            assertEquals("",root.directory);assertEquals("Fotos",root.displayPath);assertEquals(501L,root.count)
            assertEquals(listOf(8L,9L),root.preview.faces);assertEquals(key,root.preview.originalKeys[8])
            assertEquals(.3f,root.preview.faceBounds.getValue(8).width,0f)
            assertNull(root.preview.faceBounds[9]);assertTrue(9L in root.preview.reviewFaces)
            val excluded=result.folders.last()
            assertTrue(excluded.excluded);assertTrue(excluded.preview.faces.isEmpty())
            assertEquals("20240102_Family_Trip/Nested_Folder",excluded.directory)
            assertEquals("Fotos / 02.01.2024 · Family Trip / Nested Folder",excluded.displayPath)
        } finally {client.close()}
    }
    @Test fun missingDisplayPathNeverFallsBackToDirectory() = runBlocking {
        val client=client("""{"person_id":7,"name":"Ada","revision":1,"page":1,"has_next":false,"folders":[{"directory":"RAW_SECRET","count":0,"excluded":true}]}""")
        try {
            try {LabelingApi(client,"https://test.invalid/").personFolders(7,1);fail("missing label accepted")}
            catch(_:org.json.JSONException) {}
        } finally {client.close()}
    }
    @Test fun wrongPersonIsRejectedAndFolderConflictIsExplained() = runBlocking {
        val client=client("""{"person_id":8,"name":"Other","revision":1,"page":1,"has_next":false,"folders":[]}""")
        try {
            try {LabelingApi(client,"https://test.invalid/").personFolders(7,1);fail("wrong source accepted")}
            catch(_:IllegalArgumentException) {}
        } finally {client.close()}
        val conflict=client("""{"code":"folder_excluded","error":"excluded"}""",409)
        try {
            try {LabelingApi(conflict,"https://test.invalid/").action(7,"{}");fail("conflict accepted")}
            catch(e:ApiFailure) {assertEquals(R.string.people_folders_target_excluded,e.userText.resource)}
        } finally {conflict.close()}
    }
}
