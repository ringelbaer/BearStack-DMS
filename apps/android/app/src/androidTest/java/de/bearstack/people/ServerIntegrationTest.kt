package de.bearstack.people

import android.graphics.BitmapFactory
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.connection.*
import de.bearstack.people.data.local.LabelingDatabase
import de.bearstack.people.data.remote.*
import de.bearstack.people.people.PeopleRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import okhttp3.FormBody
import okhttp3.Request
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test

/** Runs only against the disposable Go fixture, never against a configured user instance. */
class ServerIntegrationTest {
    @Test fun nativeGalleryReadOnlyAgainstRealGoServerBehindPrefix() = runBlocking {
        val root=InstrumentationRegistry.getArguments().getString("labelingUrl")
        assumeTrue("Optional Go integration fixture",root=="https://127.0.0.1:18787/")
        val offer=Connections.inspect(root!!)!!
        val address=root+"gallery/"
        val client=Connections.client(Profile(address,"reader","secret",offer.encoded))
        try {
            val api=PhotosApi(client,address)
            assertFalse(api.session().canManagePeople)
            val page=api.browse(PhotoQuery(recursive=true))
            assertEquals(1,page.media.size)
            val photo=page.media.single()
            assertEquals("one.jpg",photo.path)
            assertEquals(32,api.info(photo.path).width)
            assertTrue(api.browse(PhotoQuery(query="one")).media.isNotEmpty())
            assertTrue(api.browse(PhotoQuery(),page=2,section="media").media.isEmpty())
            val blog=api.blog("story.md")
            assertTrue(blog.html.contains("<h2>Gallery story</h2>"))
            withContext(Dispatchers.IO) {
                client.newCall(Request.Builder().url(api.original(photo)).header("Range","bytes=0-9").build()).execute().use {
                    assertEquals(206,it.code);assertEquals(10,it.body!!.bytes().size)
                }
            }
            try { LabelingApi(client,address).session();fail("reader allowed to edit people") }
            catch(e:ApiFailure) {assertEquals(403,e.status)}
        } finally {Connections.close(client)}
    }
    @Test fun mergeDecisionsAndPortraitsAgainstRealGoServerBehindPrefix() = runBlocking {
        val root=InstrumentationRegistry.getArguments().getString("labelingUrl")
        assumeTrue("Optional Go integration fixture",root=="https://127.0.0.1:18787/")
        val offer=Connections.inspect(root!!)!!
        for(accept in listOf(true,false)) {
            val address=root+if(accept) "merges-accept/" else "merges-reject/"
            val client=Connections.client(Profile(address,"editor","secret",offer.encoded))
            val api=LabelingApi(client,address)
            val db=Room.inMemoryDatabaseBuilder(InstrumentationRegistry.getInstrumentation().targetContext,LabelingDatabase::class.java).build()
            try {
                val session=api.session();assertTrue(session.mergeSuggestions)
                val pair=api.nextMergeSuggestion()!!
                assertEquals("Ada",pair.target.name)
                for(person in listOf(pair.source,pair.target)) {
                    assertEquals(1,person.faces.size)
                    assertNotNull(person.faceBounds[person.faceId]);assertNotNull(person.originalKeys[person.faceId])
                    withContext(Dispatchers.IO) {
                        for(url in listOf(api.image(person.faceId),api.original(person.faceId))) {
                            client.newCall(Request.Builder().url(url).build()).execute().use {
                                assertEquals(200,it.code);assertEquals("image/jpeg",it.header("Content-Type"))
                                assertTrue(it.body!!.bytes().isNotEmpty())
                            }
                        }
                    }
                }
                val repo=PeopleRepository(db,api,session)
                repo.prepare(pair.source,if(accept) "accept_merge" else "reject_merge",target=pair.target,suggestionId=pair.id)
                val pending=repo.pending()!!
                val receipt=repo.resolve()!!
                assertEquals(receipt,api.action(pending.source,pending.body))
                assertEquals(receipt,api.receipt(pending.operation,session.dataset))
                assertNull(api.nextMergeSuggestion())
                assertEquals(if(accept) 2L else 1L,api.person(pair.target.id).count)
            } finally {db.close();Connections.close(client)}
        }
    }
    @Test fun selfSignedTlsRealGoActionsAndConcurrentWebEdits() = runBlocking {
        val address=InstrumentationRegistry.getArguments().getString("labelingUrl")
        assumeTrue("Optional Go integration fixture",address=="https://127.0.0.1:18787/")
        val offer=Connections.inspect(address!!)!!
        assertEquals(95,offer.fingerprint.length)
        // No password was needed for the certificate inspection above.
        val client=Connections.client(Profile(address,"manager","secret",offer.encoded))
        val api=LabelingApi(client,address)
        val ctx=InstrumentationRegistry.getInstrumentation().targetContext
        val db=Room.inMemoryDatabaseBuilder(ctx,LabelingDatabase::class.java).build()
        try {
            val session=api.session();val repo=PeopleRepository(db,api,session)
            val first=repo.next()!!
            assertEquals(5L,first.count);assertEquals(4,first.faces.size)
            assertEquals(1,repo.page(4).faces.size)
            val image=withContext(Dispatchers.IO) {client.newCall(Request.Builder().url(api.image(first.faces[0],true)).build()).execute().use {
                assertEquals("image/jpeg",it.header("Content-Type"));it.body!!.bytes()
            }}
            assertTrue(image.size>100)
            val original=withContext(Dispatchers.IO) {client.newCall(Request.Builder().url(api.original(first.faces[0])).build()).execute().use {
                assertEquals(200,it.code);assertEquals("image/jpeg",it.header("Content-Type"));it.body!!.bytes()
            }}
            val dimensions=BitmapFactory.Options().apply {inJustDecodeBounds=true}
            BitmapFactory.decodeByteArray(original,0,original.size,dimensions)
            assertEquals(32,dimensions.outWidth);assertEquals(32,dimensions.outHeight)
            repo.prepare(first,"detach",face=first.faces[0]);val detached=repo.resolve()!!
            assertEquals(1L,detached.faces);assertTrue(detached.newId>session.upper)
            val remaining=repo.next()!!;assertEquals(4L,remaining.count)
            // A web edit between reading the group and sending the app decision must reject it.
            withContext(Dispatchers.IO) {
                for(name in listOf("Web-Bearbeitung","")) {
                    client.newCall(Request.Builder().url(address+"photos/people/${remaining.id}/rename")
                        .post(FormBody.Builder().add("name",name).build()).build()).execute().use { assertTrue(it.code==200 || it.code==303) }
                }
            }
            repo.prepare(remaining,"name",name="Anna")
            try {repo.resolve();fail("stale decision accepted")}catch(e:ApiFailure){assertEquals(409,e.status)}
            repo.prepare(repo.next()!!,"name",name="Anna");repo.resolve()
            val priority=repo.next()!!;assertEquals(detached.newId,priority.id)
            val target=api.suggestions("Ann").single();assertEquals("Anna",target.name)
            repo.prepare(priority,"name",name="Anna")
            try {repo.resolve();fail("duplicate accepted")}catch(e:ApiFailure){assertEquals("name_exists",e.code)}
            repo.prepare(repo.next()!!,"assign",target=api.suggestions("Anna",true).single());repo.resolve()
            repo.prepare(repo.next()!!,"ignore");repo.resolve();assertNull(repo.next())
            assertEquals(5L,api.suggestions("Anna",true).single().count)
            assertTrue(api.session().namedPeople)
            assertTrue(api.session().namedSearch)
            assertEquals("Anna",api.searchPeople(0,api.session().upper,"ann").people.single().name)
            var named=api.person(api.namedPeople(0,api.session().upper).people.single().id)
            val stream=api.personFaces(named.id,0,0)
            assertEquals(5,stream.faces.size)
            assertTrue(api.personFaces(named.id,5,stream.faces.last()).faces.isEmpty())
            val favoriteFace=named.faces.first()
            repo.prepare(named,"favorite",face=favoriteFace,favorite=true);repo.resolve()
            named=api.person(named.id)
            assertTrue(favoriteFace in named.favorites)
            repo.prepare(named,"rename",name="Anna Neu");repo.resolve()
            named=api.person(named.id)
            assertEquals("Anna Neu",named.name)
            repo.prepare(named,"unassign",face=favoriteFace);val unassigned=repo.resolve()!!
            assertTrue(unassigned.sourceRevision>named.revision)
            val unnamed=repo.next()!!
            assertEquals(unassigned.newId,unnamed.id);assertEquals("",unnamed.name)
            assertTrue(unnamed.favorites.isEmpty());assertEquals(4L,api.person(named.id).count)
            val changedPin=Connections.client(Profile(address,"manager","secret","not-the-server-certificate"))
            try {LabelingApi(changedPin,address).session();fail("changed pin accepted")}catch(_:javax.net.ssl.SSLException){}
            finally {changedPin.dispatcher.executorService.shutdown();changedPin.connectionPool.evictAll()}
            val wrongPassword=Connections.client(Profile(address,"manager","wrong",offer.encoded))
            try {LabelingApi(wrongPassword,address).session();fail("wrong password accepted")}catch(e:ApiFailure){assertEquals(401,e.status)}
            finally {wrongPassword.dispatcher.executorService.shutdown();wrongPassword.connectionPool.evictAll()}
        } finally {db.close();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()}
    }
}
