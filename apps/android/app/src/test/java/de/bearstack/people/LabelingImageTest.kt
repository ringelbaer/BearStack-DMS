package de.bearstack.people

import de.bearstack.people.data.remote.LabelingApi
import okhttp3.OkHttpClient
import okhttp3.HttpUrl.Companion.toHttpUrl
import org.junit.Assert.assertEquals
import org.junit.Test
import org.json.JSONObject

class LabelingImageTest {
    @Test fun favoritesAndBrowserSearchPreserveFaceIdentityAndProxyPrefix() {
        val api=LabelingApi(OkHttpClient(),"https://example.test/bearstack/")
        val p=api.person(JSONObject("""{"id":1,"name":"Anna","revision":4,"count":3,"face_id":10,
            "faces":[{"id":10,"favorite":true},{"id":11,"favorite":false},{"id":12}]}"""))
        assertEquals(setOf(10L),p.favorites)
        val url=api.gallery("Anna & O'Neil").toHttpUrl()
        assertEquals("/bearstack/photos",url.encodedPath)
        assertEquals("person:\"Anna & O'Neil\"",url.queryParameter("q"))
        assertEquals("",url.username);assertEquals("",url.password)
        assertEquals("person:\"Anna \"'\"'\"Ace\"'\"'\" O'Neil\"",
            api.gallery("Anna \"Ace\" O'Neil").toHttpUrl().queryParameter("q"))
    }
    @Test fun normalizedBoundsStayWithTheirFaceAndLegacyResponsesRemainReadable() {
        val person=LabelingApi(OkHttpClient(),"https://example.test/").person(JSONObject("""{"id":1,"name":"","revision":3,"count":5,"face_id":10,"offset":4,
            "faces":[{"id":14,"bounds":{"x":0.1,"y":0.2,"width":0.25,"height":0.5}},
                     {"id":15}, {"id":16,"bounds":{"x":1,"y":0,"width":0,"height":0.1}},
                     {"id":17,"bounds":{"x":0,"y":0}}]}"""))
        assertEquals(setOf(14L),person.faceBounds.keys)
        val bounds=person.faceBounds.getValue(14L)
        assertEquals(.1f,bounds.x,.00001f);assertEquals(.2f,bounds.y,.00001f)
        assertEquals(.25f,bounds.width,.00001f);assertEquals(.5f,bounds.height,.00001f)
    }

    @Test fun fullDisplayPathsStayAssociatedWithTheirFaceAcrossPages() {
        val api=LabelingApi(OkHttpClient(),"https://example.test/")
        val person=api.person(JSONObject("""{"id":1,"name":"","revision":3,"count":6,"face_id":10,"offset":4,
            "faces":[{"id":14,"display_path":"Fotos / 11.05.2026 · Urlaub / IMG_1.jpg"},
                     {"id":15,"display_path":"Fotos / Archiv / IMG_2.jpg"}]}"""))
        assertEquals(listOf(14L,15L),person.faces)
        assertEquals(4,person.offset)
        assertEquals("Fotos / 11.05.2026 · Urlaub / IMG_1.jpg",person.facePaths[14])
        assertEquals("Fotos / Archiv / IMG_2.jpg",person.facePaths[15])
        val legacy=api.person(JSONObject("""{"id":1,"name":"","revision":1,"count":1,"face_id":10,"faces":[{"id":10}]}"""))
        assertEquals("",legacy.facePaths[10])
    }
    @Test fun originalUsesFaceSourceEndpointAndPreservesInstancePrefix() {
        val api = LabelingApi(OkHttpClient(), "https://example.test/bearstack/")
        assertEquals("https://example.test/bearstack/api/photos/labeling/v1/faces/59/original", api.original(59))
        assertEquals("https://example.test/bearstack/api/photos/labeling/v1/faces/59/thumbnail?size=160", api.image(59))
    }
}
