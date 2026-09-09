package de.bearstack.people

import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.PhotosController
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import org.junit.Assert.*
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class PhotosControllerTest {
    private val session=PhotoSession("scope",false,320,320,1280,2048,5,8)
    private fun photo(path: String)=Photo(path,path,"image","image/jpeg","1","2026-09-09T10:00:00Z",null,10,100,100)
    private fun page(query: PhotoQuery, page: Int=1, media: List<Photo> = emptyList(), next: Boolean=false) =
        PhotoPage(query.path,"",page,media.size,next,0,false,false,media,emptyList(),emptyList())
    private inner class Fake : PhotosService {
        var handler: suspend (PhotoQuery,Int,String) -> PhotoPage = {q,p,_ -> page(q,p)}
        var blogHandler: suspend (String)->PhotoBlog = {path -> PhotoBlog(path,path,null,"2026-09-09T10:00:00Z","Text","<p>Text</p>")}
        val requests=mutableListOf<Triple<PhotoQuery,Int,String>>()
        override suspend fun session()=session
        override suspend fun browse(query: PhotoQuery,page: Int,section: String): PhotoPage {
            requests+=Triple(query,page,section);return handler(query,page,section)
        }
        override suspend fun info(path: String)=photo(path)
        override suspend fun blog(path: String)=blogHandler(path)
        override fun thumbnail(photo: Photo,size: Int)="https://example.test/thumbnail"
        override fun original(photo: Photo)="https://example.test/media"
    }
    @Test fun textErrorsEmptyPostsAndCancelledResponsesHaveSeparateState()=runTest {
        val fake=Fake()
        val controller=PhotosController(this,fake,session);runCurrent()
        val post=PhotoBlog("notes.md","notes.md",null,"2026-09-09T10:00:00Z")
        fake.blogHandler={throw ApiFailure(404,"not_found",de.bearstack.people.text.UiText(R.string.error_missing))}
        controller.openBlog(post);runCurrent()
        assertFalse(controller.state.value.blogLoading)
        assertEquals(R.string.error_missing,controller.state.value.blogError!!.resource)
        assertNull(controller.state.value.error)
        fake.blogHandler={post}
        controller.openBlog(post);runCurrent()
        assertFalse(controller.state.value.blogLoading);assertNull(controller.state.value.blogError)
        assertEquals(post,controller.state.value.blog)
        val gate=CompletableDeferred<Unit>()
        fake.blogHandler={withContext(NonCancellable) {gate.await()};post.copy(text="late")}
        controller.openBlog(post);runCurrent()
        assertTrue(controller.state.value.blogLoading)
        controller.closeBlog();gate.complete(Unit);runCurrent()
        assertNull(controller.state.value.blog);assertNull(controller.state.value.blogError)
        assertFalse(controller.state.value.blogLoading)
        controller.close()
    }
    @Test fun lateResponseCannotReplaceNewFolderOrClosedConnection() = runTest {
        val gate=CompletableDeferred<Unit>()
        val fake=Fake().apply {handler={q,_,_ ->
            if(q.path.isEmpty()) withContext(NonCancellable) {gate.await()}
            page(q,media=listOf(photo(q.path+"/image.jpg")))
        }}
        val controller=PhotosController(this,fake,session)
        runCurrent()
        controller.open(PhotoQuery(path="new"));runCurrent()
        assertEquals("new/image.jpg",controller.state.value.media.single().path)
        gate.complete(Unit);runCurrent()
        assertEquals("new/image.jpg",controller.state.value.media.single().path)
        controller.close()
    }
    @Test fun failedPageCanBeRetriedWithoutAdvancingOrLosingVisiblePhotos() = runTest {
        var fail=true
        val fake=Fake().apply {handler={q,p,_ ->
            if(p==2 && fail) throw java.io.IOException("offline")
            page(q,p,if(p==1) listOf(photo("a")) else listOf(photo("a"),photo("b")),p==1)
        }}
        val controller=PhotosController(this,fake,session);runCurrent()
        controller.more("media");runCurrent()
        assertEquals(listOf("a"),controller.state.value.media.map {it.path})
        assertNotNull(controller.state.value.error)
        assertTrue(controller.state.value.hasNext)
        fail=false;controller.more("media");runCurrent()
        assertEquals(listOf("a","b"),controller.state.value.media.map {it.path})
        assertFalse(controller.state.value.hasNext)
        assertEquals(listOf(1,2,2),fake.requests.map {it.second})
        controller.close()
    }
    @Test fun repeatedVisibleSentinelCoalescesRequestsAndNewQueryResetsPages() = runTest {
        val gate=CompletableDeferred<Unit>()
        val fake=Fake().apply {handler={q,p,_ -> if(p==2) gate.await();page(q,p,listOf(photo("$p")),p==1)}}
        val controller=PhotosController(this,fake,session);runCurrent()
        repeat(10) {controller.more("media")};runCurrent()
        assertEquals(2,fake.requests.size)
        controller.open(PhotoQuery(query="new search"));runCurrent()
        gate.complete(Unit);runCurrent()
        assertEquals(1,fake.requests.last().second)
        assertEquals("new search",controller.state.value.query.query)
        assertEquals(listOf("1"),controller.state.value.media.map {it.path})
        controller.close()
    }
    @Test fun frameIncludesSubfoldersAndReturnsToThePreviousFolderAndPage() = runTest {
        val fake=Fake().apply {handler={q,p,_ -> page(q,p,listOf(photo("${q.path}/$p")),true)}}
        val controller=PhotosController(this,fake,session);runCurrent()
        controller.open(PhotoQuery(path="album"),tab=1);runCurrent()
        controller.more("media");runCurrent()
        controller.startFrame();runCurrent()
        assertTrue(controller.state.value.frame)
        assertTrue(fake.requests.last().first.recursive)
        assertEquals("image",fake.requests.last().first.type)
        assertNotNull(controller.state.value.selected)
        controller.closeViewer()
        assertFalse(controller.state.value.frame)
        assertEquals("album",controller.state.value.query.path)
        assertEquals(1,controller.state.value.tab)
        assertEquals(2,controller.state.value.media.size)
        controller.more("media");runCurrent()
        assertEquals(3,fake.requests.last().second)
        controller.close()
        assertTrue(controller.state.value.media.isEmpty())
    }
}
