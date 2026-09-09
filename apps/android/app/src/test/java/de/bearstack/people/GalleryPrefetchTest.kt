package de.bearstack.people

import de.bearstack.people.data.remote.Photo
import de.bearstack.people.photos.*
import de.bearstack.people.text.UiText
import org.junit.Assert.*
import org.junit.Test

class GalleryPrefetchTest {
    private fun window() = (1..3).fold(PhotoPages.media()) {pages,page ->
        pages.add(page,List(96) {
            val path="image-${(page-1)*96+it}"
            Photo(path,path,"image","image/jpeg","1","2026-09-09T10:00:00Z",null,1,80,80)
        },true)
    }
    @Test fun wholeWindowVisibleCannotCauseAnEvictionReloadLoop() {
        val pages=window()
        val state=PhotosState(mediaPages=pages)
        val all=pages.keys.mapTo(HashSet()) {"photo:$it"}
        assertNull(galleryPrefetch(state,all))
        assertEquals(GalleryPrefetch("media",false),galleryPrefetch(state,setOf("photo:image-275")))
        assertNull(galleryPrefetch(state,setOf("photo:image-130")))
    }
    @Test fun errorsAndHiddenGalleryStopAutomaticRetries() {
        val state=PhotosState(mediaPages=window())
        val keys=setOf("photo:image-275")
        assertNull(galleryPrefetch(state.copy(selected="image-275"),keys))
        assertNull(galleryPrefetch(state.copy(frame=true),keys))
        assertNull(galleryPrefetch(state.copy(loadingSections=setOf("media")),keys))
        assertNull(galleryPrefetch(state.copy(pageErrors=mapOf("media" to PhotoPageFailure(false,UiText(R.string.photos_map_error)))),keys))
        assertNull(galleryPrefetch(state.copy(scrollToKey="photo:image-275"),keys))
    }
}
