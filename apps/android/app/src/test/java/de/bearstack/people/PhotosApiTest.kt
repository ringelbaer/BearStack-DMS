package de.bearstack.people

import de.bearstack.people.data.remote.*
import kotlinx.coroutines.runBlocking
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.*
import org.junit.Test

class PhotosApiTest {
    @Test fun frameCaptionUsesServerLabelsAndNeverDerivesRawPathsOnOlderServers()=runBlocking {
        var folder="\"folder_name\":\"Sommer Urlaub\","
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body("""{"media":{"path":"2026_07_15_Sommer_Urlaub/a.jpg","name":"a.jpg",$folder"type":"image","mime":"image/jpeg","version":"1","modified":"2026-09-09T10:00:00Z","bytes":1,"width":1,"height":1}}""".toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val api=PhotosApi(client,"https://example.test/")
            val settings=de.bearstack.people.photos.PlaybackSettings(frameFolderName=true)
            val photo=api.info("2026_07_15_Sommer_Urlaub/a.jpg")
            assertEquals("Sommer Urlaub",de.bearstack.people.photos.frameCaption(photo,settings,"Unavailable"))
            assertEquals("a.jpg",de.bearstack.people.photos.frameCaption(photo,settings.copy(frameFolderName=false),"Unavailable"))
            folder=""
            val legacy=api.info(photo.path)
            assertEquals("Unavailable",de.bearstack.people.photos.frameCaption(legacy,settings,"Unavailable"))
            assertEquals(photo.path,legacy.path)
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }

    @Test fun peopleCountSortingRequiresAnExplicitSessionCapability()=runBlocking {
        var capability=""
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body("""{"protocol":1,"instance":"test","dataset":"photos","account":"reader","can_manage_people":false,$capability
                    "settings":{"thumbnail_size":240,"folder_thumbnail_size":240,"preview_size":1280,"large_preview_size":2048,"slideshow_seconds":5,"frame_seconds":8}}"""
                    .toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val api=PhotosApi(client,"https://example.test/")
            assertFalse(api.session().peopleCountSort)
            capability="\"people_count_sort\":true,"
            assertTrue(api.session().peopleCountSort)
            capability="\"people_count_sort\":false,"
            assertFalse(api.session().peopleCountSort)
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun virtualPeopleFoldersUseCachedFacesAndKeepProxyPrefix()=runBlocking {
        var request:Request?=null
        val portrait="""{"path":"","name":"Zoe","type":"image","mime":"image/jpeg","version":"0","modified":"2026-09-16T00:00:00Z","bytes":0,"width":0,"height":0,"face_id":42}"""
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            request=chain.request()
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body("""{"path":".people","parent":"","name":"Personen","page":1,"total":0,"has_next":false,"folder_total":1,"folder_has_next":false,"blog_has_next":false,"media":[],"blogs":[],"folders":[{"path":".people/all","name":"Alle","media_count":0,"folder_count":8,"approximate":false,"virtual":true,"previews":[${List(8){portrait}.joinToString(",")}]}]}""".toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val api=PhotosApi(client,"https://example.test/proxy/")
            val page=api.browse(PhotoQuery(path=".people"))
            assertEquals("1",request!!.url.queryParameter("people"))
            assertEquals("Personen",page.name);assertEquals("",page.parent)
            assertTrue(page.folders.single().virtual);assertEquals(8,page.folders.single().previews.size)
            assertEquals("https://example.test/proxy/api/photos/v1/faces/42/thumbnail",api.thumbnail(page.folders.single().previews.first(),320))
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }

    @Test fun folderPeopleNavigationIsOptionalAndKeepsOpaquePaths()=runBlocking {
        var body="""{"path":"Holiday","parent":"","people_path":".people/f-SG9saWRheQ","page":1,"total":0,"has_next":false,"folder_total":0,"folder_has_next":false,"blog_has_next":false,"media":[],"blogs":[],"folders":[]}"""
        val client=OkHttpClient.Builder().addInterceptor {chain -> Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK").body(body.toResponseBody("application/json".toMediaType())).build() }.build()
        try {
            val api=PhotosApi(client,"https://example.test/proxy/")
            assertEquals(".people/f-SG9saWRheQ",api.browse(PhotoQuery(path="Holiday")).peoplePath)
            body=body.replace("\"people_path\":\".people/f-SG9saWRheQ\",", "")
            assertEquals("",api.browse(PhotoQuery(path="Holiday")).peoplePath)
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }

    @Test fun datePositionKeepsProxyPrefixAndUsesABoundedValidatedResponse()=runBlocking {
        var request:Request?=null
        var body="""{"path":"album/a.jpg","date":"2026-06-12","page":321}"""
        var status=200
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            request=chain.request()
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(status).message("response")
                .body(body.toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val api=PhotosApi(client,"https://example.test/proxy/")
            val target=api.locateDate("2026-06-11")
            assertEquals(321,target.page);assertEquals("2026-06-12",target.date)
            assertEquals("/proxy/api/photos/v1/browse/date",request!!.url.encodedPath)
            assertEquals("2026-06-11",request!!.url.queryParameter("date"))
            body="""{"path":"a","date":"2026-02-30","page":0}"""
            try {api.locateDate("2026-06-11");fail("invalid date position accepted")}
            catch(_:IllegalArgumentException) {}
            status=404;body="""{"code":"not_found"}"""
            try {api.locateDate("2026-06-11");fail("missing endpoint accepted")}
            catch(e:de.bearstack.people.text.UserIoFailure) {assertEquals(R.string.photos_date_unavailable,e.userText.resource)}
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun photoRouteKeepsSearchPrefixAndFullCountsWithBoundedGeometry()=runBlocking {
        var request:Request?=null
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            request=chain.request()
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body("""{"path":"trip","name":"","total_media":200000,"radius_meters":500,"total_points":150000,
                    "segments":[[[1,2],[3,4]]],"simplified":true,"omitted_segments":0}""".toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val route=PhotosApi(client,"https://example.test/proxy/").route(PhotoQuery(path="trip",query="tag:sea",type="image"),null,4096)
            assertEquals(200000,route.totalMedia);assertEquals(150000,route.geometry.totalPoints)
            assertTrue(route.geometry.simplified);assertEquals(2,route.geometry.segments.single().size)
            assertEquals("/proxy/api/photos/v1/map/route",request!!.url.encodedPath)
            assertEquals("tag:sea",request!!.url.queryParameter("q"));assertEquals("image",request!!.url.queryParameter("type"))
            assertEquals("4096",request!!.url.queryParameter("points"))
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun infoKeepsSourceTimeRatingAndNamesFromBothMetadataFormats() = runBlocking {
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .body("""{"media":{"path":"trip/a.jpg","name":"a.jpg","type":"image","mime":"image/jpeg","version":"1",
                    "modified":"2026-09-09T10:00:00Z","captured":"2024-01-02T00:15:30+14:00","bytes":1024,"width":400,"height":300,
                    "rating":4.5,"tags":["Trip","Trip"],"keywords":["Sea"," ","Sun"],
                    "faces":[{"Name":"Alex"}],"automatic_faces":[{"name":"Alex"},{"name":"Sam"},{"name":""}]}}"""
                    .toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val photo=PhotosApi(client,"https://example.test/").info("trip/a.jpg")
            assertEquals("2024-01-02T00:15:30+14:00",photo.date)
            assertEquals(4.5,photo.rating!!,0.0)
            assertEquals(listOf("Trip"),photo.tags)
            assertEquals(listOf("Sea","Sun"),photo.keywords)
            assertEquals(listOf("Alex","Sam",""),photo.people)
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
    @Test fun trackRequestsKeepPrefixCursorAndSegmentGaps() = runBlocking {
        val requests=mutableListOf<Request>()
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            val request=chain.request();requests+=request
            val body=if(request.url.encodedPath.endsWith("/tracks"))
                """{"tracks":[{"path":"trip & sea/route.gpx","name":"route.gpx","modified":"2026-09-09T00:00:00Z","bytes":42}],"cursor":"64","previous_cursor":"33","has_next":true,"has_previous":true,"ready":true}"""
            else """{"path":"trip & sea/route.gpx","name":"route.gpx","segments":[[[1,179],[2,-179]],[[3,5]]],"total_points":3,"simplified":false,"omitted_segments":0}"""
            Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(200).message("OK").body(body.toResponseBody("application/json".toMediaType())).build()
        }.build()
        try {
            val api=PhotosApi(client,"https://example.test/proxy/")
            val files=api.tracks("trip & sea","65",true)
            assertEquals(1,files.tracks.size);assertEquals("33",files.previousCursor)
            assertEquals("/proxy/api/photos/v1/map/tracks",requests[0].url.encodedPath)
            assertEquals("65",requests[0].url.queryParameter("cursor"));assertEquals("1",requests[0].url.queryParameter("before"))
            val track=api.track(files.tracks.single().path,PhotoMapBounds(-10.0,170.0,10.0,-170.0),32)
            assertEquals(listOf(2,1),track.segments.map {it.size})
            assertEquals("/proxy/api/photos/v1/map/track",requests[1].url.encodedPath)
            assertEquals("trip & sea/route.gpx",requests[1].url.queryParameter("path"))
            assertEquals("32",requests[1].url.queryParameter("points"));assertEquals("-170.0",requests[1].url.queryParameter("east"))
        } finally {client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
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
