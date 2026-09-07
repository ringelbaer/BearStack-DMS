package de.bearstack.people

import de.bearstack.people.data.remote.LabelingApi
import okhttp3.OkHttpClient
import org.junit.Assert.assertEquals
import org.junit.Test
import org.json.JSONObject

class LabelingImageTest {
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
