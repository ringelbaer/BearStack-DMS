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
        assertNotNull(controller.state.value.pageErrors["media"])
        assertNull(controller.state.value.error)
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

    @Test fun largeCollectionsKeepThreePagesPerSectionAndReloadEvictedPagesInOrder()=runTest {
        val fake=Fake().apply {handler={q,p,section ->
            val media=if(section=="" || section=="media") List(96) {photo("image-${(p-1)*96+it}")} else emptyList()
            val folders=if(section=="" || section=="folders") List(24) {
                PhotoFolder("folder-${(p-1)*24+it}","Folder",null,100,false,0,media.take(2))
            } else emptyList()
            val blogs=if(section=="" || section=="blogs") List(20) {
                PhotoBlog("post-${(p-1)*20+it}.md","Post",null,"2026-09-09T10:00:00Z")
            } else emptyList()
            PhotoPage(q.path,"",p,1_000_000,true,100_000,true,true,media,folders,blogs)
        }}
        val controller=PhotosController(this,fake,session);runCurrent()
        repeat(249) {
            for(section in listOf("media","folders","blogs")) controller.more(section)
            runCurrent()
            val s=controller.state.value
            assertTrue(s.media.size<=288);assertTrue(s.folders.size<=72);assertTrue(s.blogs.size<=60)
        }
        assertEquals(248,controller.state.value.mediaPages.firstPage)
        assertEquals(250,controller.state.value.mediaPages.lastPage)
        val expectedFirst="image-${247*96}"
        assertEquals(expectedFirst,controller.state.value.media.first().path)
        assertEquals(247*96+1,controller.state.value.mediaPages.position(expectedFirst))
        repeat(249) {
            for(section in listOf("media","folders","blogs")) controller.previous(section)
            runCurrent()
            val s=controller.state.value
            assertTrue(s.media.size<=288);assertTrue(s.folders.size<=72);assertTrue(s.blogs.size<=60)
        }
        assertFalse(controller.state.value.mediaPages.hasPrevious)
        assertEquals((0 until 288).map {"image-$it"},controller.state.value.media.map {it.path})
        assertEquals((0 until 72).map {"folder-$it"},controller.state.value.folders.map {it.path})
        assertEquals((0 until 60).map {"post-$it.md"},controller.state.value.blogs.map {it.path})
        controller.close()
    }

    @Test fun viewerNeighboursSurviveEvictionFailureReverseAndRepeat()=runTest {
        var fail=false
        val fake=Fake().apply {handler={q,p,_ ->
            if(fail) throw java.io.IOException("offline")
            page(q,p,List(96) {photo("image-${(p-1)*96+it}")},p<5).copy(total=480)
        }}
        val controller=PhotosController(this,fake,session);runCurrent()
        for(p in 2..4) {controller.more("media");runCurrent()}
        assertEquals(2,controller.state.value.mediaPages.firstPage)
        assertEquals("image-384",controller.neighbour("image-383",1)?.path)
        assertEquals(3,controller.state.value.mediaPages.firstPage)
        assertEquals(385,controller.state.value.mediaPages.position("image-384"))
        fail=true
        assertNull(controller.neighbour("image-192",-1))
        assertTrue(controller.state.value.pageErrors.getValue("media").previous)
        assertEquals("image-192",controller.state.value.media.first().path)
        fail=false
        assertEquals("image-191",controller.neighbour("image-192",-1)?.path)
        assertTrue(controller.state.value.pageErrors.isEmpty())
        assertEquals(2,controller.state.value.mediaPages.firstPage)
        controller.more("media");runCurrent()
        assertNull(controller.neighbour("image-479",1))
        assertEquals("image-0",controller.neighbour("image-479",1,repeat=true)?.path)
        assertEquals(1,controller.state.value.mediaPages.firstPage)
        assertEquals(96,controller.state.value.media.size)
        controller.close()
    }

    @Test fun latePrefetchCannotEvictTheNewViewportOrOpenAnEvictedPhoto()=runTest {
        val gate=CompletableDeferred<Unit>()
        val fake=Fake().apply {handler={q,p,_ ->
            if(p==4) gate.await()
            page(q,p,List(96) {photo("image-${(p-1)*96+it}")},true)
        }}
        val controller=PhotosController(this,fake,session);runCurrent()
        repeat(2) {controller.more("media");runCurrent()}
        controller.galleryVisible(setOf("photo:image-287"))
        controller.more("media");runCurrent()
        controller.galleryVisible(setOf("photo:image-0"))
        gate.complete(Unit);runCurrent()
        assertEquals(1,controller.state.value.mediaPages.firstPage)
        assertEquals(3,controller.state.value.mediaPages.lastPage)
        assertTrue(controller.state.value.loadingSections.isEmpty())
        controller.galleryVisible(setOf("photo:image-287"))
        controller.select("image-0")
        controller.more("media");runCurrent()
        assertEquals(1,controller.state.value.mediaPages.firstPage)
        controller.select(null)
        controller.more("media");runCurrent()
        assertEquals(2,controller.state.value.mediaPages.firstPage)
        controller.select("image-0")
        assertNull(controller.state.value.selected)
        controller.close()
        assertNull(controller.gridPosition)
    }

    @Test fun backwardFailureRetriesItsPageAndLateResponsesCannotRestoreOldCollections()=runTest {
        var fail=false
        val gate=CompletableDeferred<Unit>()
        var delayed=false
        val fake=Fake().apply {handler={q,p,_ ->
            if(delayed && q.path.isEmpty()) withContext(NonCancellable) {gate.await()}
            if(fail) throw java.io.IOException("offline")
            page(q,p,List(96) {photo("${q.path}/image-${(p-1)*96+it}")},true)
        }}
        val controller=PhotosController(this,fake,session);runCurrent()
        repeat(4) {controller.more("media");runCurrent()}
        fail=true
        controller.previous("media");runCurrent()
        assertEquals(3,controller.state.value.mediaPages.firstPage)
        fail=false
        controller.retryPage("media");runCurrent()
        assertEquals(2,controller.state.value.mediaPages.firstPage)
        assertEquals(listOf(2,2),fake.requests.takeLast(2).map {it.second})
        delayed=true
        controller.previous("media");runCurrent()
        controller.open(PhotoQuery(path="new"));runCurrent()
        gate.complete(Unit);runCurrent()
        assertEquals("new/image-0",controller.state.value.media.first().path)
        assertFalse(controller.state.value.mediaPages.hasPrevious)
        assertTrue(controller.state.value.pageErrors.isEmpty())
        controller.close()
    }
}
